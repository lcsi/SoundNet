/*
 * Audio Bridge - Cross-platform miniaudio implementation
 *
 * miniaudio is a single-file C library for audio playback.
 * It handles decoding (MP3, FLAC, Vorbis, WAV) and output
 * through platform-specific backends:
 *   - Android: OpenSL ES (loaded via dlopen at runtime)
 *   - Linux: ALSA, PulseAudio
 *   - Windows: WASAPI, DirectSound
 *
 * The MINIAUDIO_IMPLEMENTATION macro must be defined exactly
 * once across all compilation units.
 *
 * Platform backend selection is done via CFLAGS at compile time:
 *   -DMA_ENABLE_OPENSL   (Android)
 *   -DMA_ENABLE_ALSA     (Linux)
 *   -DMA_ENABLE_WASAPI   (Windows)
 *
 * If no specific backend is enabled, miniaudio will auto-detect
 * the best available backend for the current platform.
 */

/*
 * Platform-specific backend configuration.
 * These defines should be passed via CFLAGS at compile time.
 * Uncomment the appropriate line for your platform if not using CFLAGS.
 */
// #define MA_ENABLE_OPENSL    // Android
// #define MA_ENABLE_ALSA      // Linux
// #define MA_ENABLE_WASAPI    // Windows

/*
 * If any specific backend is enabled, we disable auto-detection
 * and only compile the requested backend(s).
 */
#if defined(MA_ENABLE_OPENSL) || defined(MA_ENABLE_ALSA) || defined(MA_ENABLE_WASAPI)
#define MA_ENABLE_ONLY_SPECIFIC_BACKENDS
#endif

#define MINIAUDIO_IMPLEMENTATION
#include "miniaudio.h"

#include <stdio.h>
#include <string.h>
#include "bridge.h"

/* ==========================================================================
 * Windows Power Request - 阻止系统进入睡眠/待机
 * ========================================================================== */
#ifdef _WIN32

#include <windows.h>
#include <powrprof.h>

// 链接 powrprof.lib
#pragma comment(lib, "powrprof.lib")

static HANDLE hPowerRequest = NULL;

static void power_request_init(void) {
    // 创建电源请求上下文
    REASON_CONTEXT context;
    context.Version = POWER_REQUEST_CONTEXT_VERSION;
    context.Flags = POWER_REQUEST_CONTEXT_SIMPLE_STRING;
    context.Reason.SimpleReasonString = (LPWSTR)L"Audio playback in progress";
    
    hPowerRequest = PowerCreateRequest(&context);
    if (!hPowerRequest || hPowerRequest == INVALID_HANDLE_VALUE) {
        fprintf(stderr, "[Audio] PowerCreateRequest failed (error %lu)\n", GetLastError());
        hPowerRequest = NULL;
        return;
    }
    
    // 请求阻止系统睡眠（屏幕可以关闭）
    PowerSetRequest(hPowerRequest, PowerRequestSystemRequired);
    
    fprintf(stderr, "[Audio] Power request activated - system sleep blocked\n");
}

static void power_request_uninit(void) {
    if (hPowerRequest) {
        PowerClearRequest(hPowerRequest, PowerRequestSystemRequired);
        CloseHandle(hPowerRequest);
        fprintf(stderr, "[Audio] Power request released\n");
        hPowerRequest = NULL;
    }
}
#else
// 非 Windows 平台的空实现
static void power_request_init(void) {}
static void power_request_uninit(void) {}
#endif

/* ==========================================================================
 * CGO callback bridge
 *
 * registerGoCallback / clearGoCallback are called from Go (engine.go) to
 * register the //export'd Go function goAudioFinishedCallback as the
 * miniaudio end-of-playback callback.  These must live in a .c file (not
 * a CGO preamble) to avoid duplicate-symbol linker errors.
 * ========================================================================== */

extern void goAudioFinishedCallback(void);

void registerGoCallback(void) {
    audio_set_finished_callback((audio_finished_callback)goAudioFinishedCallback);
}

void clearGoCallback(void) {
    audio_set_finished_callback(NULL);
}

/* ==========================================================================
 * Internal State
 * ========================================================================== */

typedef struct {
    ma_context   context;           // Audio context (must outlive engine)
    ma_device    device;            // Audio device (must outlive engine)
    ma_engine    engine;            // High-level audio engine
    ma_sound     sound;             // Currently loaded sound
    bool         engine_initialized;
    bool         sound_loaded;
    bool         is_playing;        // Track if we were playing before device change
    float        current_volume;    // Remember volume across reinit
    float        last_position;     // Remember position across reinit
    char         current_file[1024]; // Remember current file path
    audio_finished_callback finished_cb;
    volatile bool device_change_pending; // Flag: device change needs handling
} AudioContext;

static AudioContext ctx = {0};

/* ==========================================================================
 * Internal Helpers
 * ========================================================================== */

/*
 * Device notification callback - fires when device state changes
 * (started, stopped, rerouted, etc.)
 *
 * IMPORTANT: We cannot reinitialize the engine/device inside this callback.
 * Instead, we set a flag and let audio_update() handle the reinit.
 */
static void on_device_notification(const ma_device_notification* pNotification) {
    if (pNotification == NULL) return;

    switch (pNotification->type) {
        case ma_device_notification_type_rerouted:
            // Device was rerouted (e.g., user switched audio output device)
            fprintf(stderr, "[Audio] Device rerouted notification received\n");
            ctx.device_change_pending = true;
            break;
        case ma_device_notification_type_stopped:
            // Device stopped (e.g., unplugged)
            fprintf(stderr, "[Audio] Device stopped notification received\n");
            ctx.device_change_pending = true;
            break;
        case ma_device_notification_type_started:
            fprintf(stderr, "[Audio] Device started notification received\n");
            break;
        default:
            fprintf(stderr, "[Audio] Device notification type: %d\n", pNotification->type);
            break;
    }
}

/*
 * Callback invoked by miniaudio when a sound finishes playing.
 * This runs on miniaudio's internal audio thread.
 */
static void on_sound_end_callback(void* pUserData, ma_sound* pSound) {
    (void)pUserData;
    (void)pSound;

    if (ctx.finished_cb) {
        ctx.finished_cb();
    }
}

// 1. 定义数据回调：用于从引擎读取 PCM 数据并写入声卡
void engine_data_callback(ma_device* pDevice, void* pOutput, const void* pInput, ma_uint32 frameCount)
{
    ma_engine* pEngine = (ma_engine*)pDevice->pUserData;
    if (pEngine == NULL) return;
    
    // 调用引擎 API 渲染数据
    ma_engine_read_pcm_frames(pEngine, pOutput, frameCount, NULL);
    (void)pInput;
}

/* ==========================================================================
 * Public API Implementation
 * ========================================================================== */

int audio_init(void) {
    if (ctx.engine_initialized) {
        fprintf(stderr, "[Audio] audio_init: already initialized\n");
        return 0;  // Already initialized
    }

    // Reset state
    memset(&ctx, 0, sizeof(ctx));

    // 方式1
    // ma_engine_config config = ma_engine_config_init();
    // // miniaudio will auto-detect the best backend for the current platform
    // // based on the MA_ENABLE_* defines passed at compile time.
    // config.noAutoStart = MA_FALSE;

    // // Set device notification callback to handle device changes
    // config.notificationCallback = on_device_notification;

    // ma_result result = ma_engine_init(&config, &ctx.engine);
    // if (result != MA_SUCCESS) {
    //     fprintf(stderr, "[Audio] ma_engine_init failed: result=%d\n", result);
    //     return -1;
    // }

    // 方式2
    // 1. 初始化上下文（Context）
    ma_context_config contextConfig = ma_context_config_init();
    contextConfig.threadPriority = ma_thread_priority_realtime; // 提升全局线程优先级
    
    ma_result result = ma_context_init(NULL, 0, &contextConfig, &ctx.context);
    if (result != MA_SUCCESS) {
      fprintf(stderr, "[Audio] ma_context_init failed: result=%d\n", result);
      return -1;
    }
    
    // 2. 配置并初始化底层设备（Device）
    // 注意：引擎内部默认期望设备使用标准的样本格式（如 ma_format_f32）
    ma_device_config deviceConfig = ma_device_config_init(ma_device_type_playback);
    deviceConfig.playback.format    = ma_format_f32; // 建议设置为 f32，因为引擎内部以浮点运算为主
    deviceConfig.playback.channels  = 2;            // 双声道
    deviceConfig.sampleRate         = 48000;        // 目标采样率
    deviceConfig.dataCallback       = engine_data_callback; // 必须绑定引擎的数据回调
    deviceConfig.pUserData          = &ctx.engine;            // 必须将引擎指针作为用户数据传入
    
    // WASAPI 音频配置优化
    // 注意：pro_audio + exclusive 需要系统权限，大多数声卡不支持
    // 使用共享模式 + 低延迟配置作为折中
    deviceConfig.wasapi.usage = ma_wasapi_usage_games;//ma_wasapi_usage_playback;  // 使用播放模式而非 pro_audio
    
    // 使用共享模式（兼容性更好）
    deviceConfig.playback.shareMode = ma_share_mode_shared;
    
    // 增大缓冲区防止熄屏卡顿
    deviceConfig.periodSizeInFrames = 960;   // 20ms @ 48kHz
    deviceConfig.periods = 6;                // 6个周期 = 120ms 缓冲
    
    result = ma_device_init(&ctx.context, &deviceConfig, &ctx.device);
    if (result != MA_SUCCESS) {
      fprintf(stderr, "[Audio] ma_device_init failed: result=%d\n", result);
      ma_context_uninit(&ctx.context);
      return -1;
    }
    
    // 3. 配置并初始化高级引擎（Engine）
    ma_engine_config config = ma_engine_config_init();
    config.pDevice              = &ctx.device; // 将上面配置好的专业音频设备挂载到引擎
    config.notificationCallback = on_device_notification;
    config.noAutoStart          = MA_FALSE;
    
    result = ma_engine_init(&config, &ctx.engine);
    if (result != MA_SUCCESS) {
      fprintf(stderr, "[Audio] ma_engine_init failed: result=%d\n", result);
      ma_device_uninit(&ctx.device);
      ma_context_uninit(&ctx.context);
      return -1;
    }

    ctx.engine_initialized = true;
    
    // 启动电源请求，阻止系统睡眠
    power_request_init();
    
    fprintf(stderr, "[Audio] audio_init: success\n");
    return 0;
}

int audio_load(const char* filepath) {
    if (!ctx.engine_initialized) {
        fprintf(stderr, "[Audio] audio_load failed: engine not initialized\n");
        return -1;
    }

    fprintf(stderr, "[Audio] audio_load: '%s'\n", filepath ? filepath : "(null)");

    // Save file path for potential reinit on device change
    if (filepath) {
        strncpy(ctx.current_file, filepath, sizeof(ctx.current_file) - 1);
        ctx.current_file[sizeof(ctx.current_file) - 1] = '\0';
        fprintf(stderr, "[Audio] Saved file path: '%s'\n", ctx.current_file);
    }

    // Stop and unload any previously loaded sound
    if (ctx.sound_loaded) {
        ma_sound_stop(&ctx.sound);
        ma_sound_uninit(&ctx.sound);
        ctx.sound_loaded = false;
    }

    // Load the audio file
    // MA_SOUND_FLAG_NO_PITCH | MA_SOUND_FLAG_NO_SPATIALIZATION: disable
    // features we don't need, saving some CPU overhead.
    ma_result result = ma_sound_init_from_file(
        &ctx.engine,
        filepath,
        MA_SOUND_FLAG_NO_PITCH | MA_SOUND_FLAG_NO_SPATIALIZATION,
        NULL,   // No listening node (2D sound)
        NULL,   // No custom data
        &ctx.sound
    );

    if (result != MA_SUCCESS) {
        fprintf(stderr, "[Audio] ma_sound_init_from_file failed: result=%d\n", result);
        return -1;
    }

    // Register end-of-playback callback
    ma_sound_set_end_callback(&ctx.sound, on_sound_end_callback, NULL);

    ctx.sound_loaded = true;
    return 0;
}

void audio_play(void) {
    if (!ctx.sound_loaded) return;
    ctx.is_playing = true;
    ma_sound_start(&ctx.sound);
}

void audio_pause(void) {
    if (!ctx.sound_loaded) return;
    ctx.is_playing = false;
    ma_sound_stop(&ctx.sound);
}

void audio_stop(void) {
    if (!ctx.sound_loaded) return;
    ctx.is_playing = false;
    ma_sound_stop(&ctx.sound);
    ma_sound_seek_to_pcm_frame(&ctx.sound, 0);
}

void audio_seek(float seconds) {
    if (!ctx.sound_loaded) return;

    ma_uint64 sampleRate = ma_engine_get_sample_rate(&ctx.engine);
    ma_uint64 targetFrame = (ma_uint64)((double)seconds * (double)sampleRate);

    ma_sound_seek_to_pcm_frame(&ctx.sound, targetFrame);
}

void audio_set_volume(float volume) {
    if (!ctx.sound_loaded) return;

    // Clamp to [0, 1]
    if (volume < 0.0f) volume = 0.0f;
    if (volume > 1.0f) volume = 1.0f;

    // Save volume for potential reinit
    ctx.current_volume = volume;

    ma_sound_set_volume(&ctx.sound, volume);
}

float audio_get_position(void) {
    if (!ctx.sound_loaded) return 0.0f;

    ma_uint64 frames = ma_sound_get_time_in_pcm_frames(&ctx.sound);
    ma_uint64 sampleRate = ma_engine_get_sample_rate(&ctx.engine);

    if (sampleRate == 0) return 0.0f;
    return (float)((double)frames / (double)sampleRate);
}

float audio_get_duration(void) {
    if (!ctx.sound_loaded) return 0.0f;

    ma_uint64 frames;
    ma_result result = ma_sound_get_length_in_pcm_frames(&ctx.sound, &frames);
    if (result != MA_SUCCESS) return 0.0f;

    ma_uint64 sampleRate = ma_engine_get_sample_rate(&ctx.engine);
    if (sampleRate == 0) return 0.0f;
    return (float)((double)frames / (double)sampleRate);
}

bool audio_is_playing(void) {
    if (!ctx.sound_loaded) return false;
    return ma_sound_is_playing(&ctx.sound);
}

void audio_update(void) {
    // Check if a device change was detected and needs handling
    if (!ctx.device_change_pending) {
        return;
    }

    fprintf(stderr, "[Audio] Device change pending, handling reinit...\n");

    // Clear the flag first to avoid re-entry
    ctx.device_change_pending = false;

    // Only reinit if we have a valid state to restore
    if (!ctx.engine_initialized || ctx.current_file[0] == '\0') {
        fprintf(stderr, "[Audio] Cannot reinit: engine=%d, file='%s'\n",
                ctx.engine_initialized, ctx.current_file);
        return;
    }

    // Remember current state before destroying
    bool was_playing = ctx.is_playing;
    float saved_volume = ctx.current_volume;
    float saved_position = 0.0f;
    char saved_file[1024];
    strncpy(saved_file, ctx.current_file, sizeof(saved_file) - 1);
    saved_file[sizeof(saved_file) - 1] = '\0';

    if (ctx.sound_loaded) {
        saved_position = (float)((double)ma_sound_get_time_in_pcm_frames(&ctx.sound) /
                                (double)ma_engine_get_sample_rate(&ctx.engine));
    }

    fprintf(stderr, "[Audio] Reinit: was_playing=%d, volume=%.2f, position=%.2f, file='%s'\n",
            was_playing, saved_volume, saved_position, saved_file);

    // Destroy current engine
    audio_destroy();

    // Reinitialize
    if (audio_init() == 0) {
        fprintf(stderr, "[Audio] Engine reinit OK, reloading file...\n");
        int load_result = audio_load(saved_file);
        if (load_result == 0) {
            audio_set_volume(saved_volume);
            audio_seek(saved_position);
            if (was_playing) {
                fprintf(stderr, "[Audio] Resuming playback...\n");
                audio_play();
            }
        } else {
            fprintf(stderr, "[Audio] Failed to reload file: '%s', error=%d\n", saved_file, load_result);
        }
    } else {
        fprintf(stderr, "[Audio] Failed to reinit engine\n");
    }
}

void audio_set_finished_callback(audio_finished_callback cb) {
    ctx.finished_cb = cb;
}

void audio_destroy(void) {
    if (ctx.sound_loaded) {
        ma_sound_stop(&ctx.sound);
        ma_sound_uninit(&ctx.sound);
        ctx.sound_loaded = false;
    }
    if (ctx.engine_initialized) {
        ma_engine_uninit(&ctx.engine);
        ctx.engine_initialized = false;
    }
    // 注意：engine_uninit 会停止但不会释放我们提供的 device
    // 我们需要手动清理 device 和 context
    ma_device_uninit(&ctx.device);
    ma_context_uninit(&ctx.context);
    
    // 释放电源请求
    power_request_uninit();
}
