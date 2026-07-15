package alarm

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"music-player/channel"
	"music-player/db"
	"music-player/models"
	redislib "music-player/redis"
)

// Engine manages all alarm timers with a polling goroutine.
type Engine struct {
	redis      *redislib.Client
	db         *db.DB
	channelMgr *channel.Manager

	// Callbacks for executing alarm actions
	OnAlarmStart   func(alarm *models.Alarm, ch *channel.Channel)
	OnSleepTimer   func(alarm *models.Alarm, ch *channel.Channel)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex

	// Track active auto-stop associations: alarm_start_id -> sleep_timer_alarm_id
	activeStops map[string]string
}

// New creates a new alarm engine.
func New(r *redislib.Client, d *db.DB, cm *channel.Manager) *Engine {
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		redis:       r,
		db:          d,
		channelMgr:  cm,
		ctx:         ctx,
		cancel:      cancel,
		activeStops: make(map[string]string),
	}
}

// SetCallbacks sets the action callbacks for alarm triggers.
func (e *Engine) SetCallbacks(onStart func(*models.Alarm, *channel.Channel), onSleep func(*models.Alarm, *channel.Channel)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.OnAlarmStart = onStart
	e.OnSleepTimer = onSleep
}

// Start begins the polling loop and loads existing alarms.
func (e *Engine) Start() {
	log.Println("[AlarmEngine] Starting...")

	// Load all enabled alarms from DB and schedule them
	e.loadExistingAlarms()

	// Start the polling goroutine
	e.wg.Add(1)
	go e.pollLoop()

	log.Println("[AlarmEngine] Started")
}

// Stop gracefully stops the engine.
func (e *Engine) Stop() {
	log.Println("[AlarmEngine] Stopping...")
	e.cancel()
	e.wg.Wait()
	log.Println("[AlarmEngine] Stopped")
}

// loadExistingAlarms loads all enabled alarms from DB and (re)schedules them into Redis.
func (e *Engine) loadExistingAlarms() {
	if e.db == nil {
		return
	}

	alarms, err := e.db.ListAllEnabledAlarms()
	if err != nil {
		log.Printf("[AlarmEngine] Failed to load alarms from DB: %v", err)
		return
	}

	ctx := context.Background()
	for _, alarm := range alarms {
		// Handle song-count triggers separately (no time-based scheduling)
		if alarm.Type == models.AlarmTypeSleepTimer && alarm.TriggerMode == models.AlarmTriggerSongCount {
			if e.redis != nil && alarm.SongCount > 0 {
				e.redis.SetAlarmHash(ctx, alarm)
				e.redis.AddChannelAlarm(ctx, alarm.Channel, alarm.ID)
				e.redis.SetRemainingSongs(ctx, alarm.ID, alarm.SongCount)
				e.redis.AddActiveSongStop(ctx, alarm.Channel, alarm.ID)
				log.Printf("[AlarmEngine] Loaded song-count sleep timer %s: trigger after %d songs", alarm.ID, alarm.SongCount)
			}
			continue
		}

		// Schedule next trigger time considering repeat rules
		nextTrigger := e.calcNextTrigger(alarm)
		if nextTrigger > 0 {
			// Store in Redis
			if e.redis != nil {
				e.redis.SetAlarmHash(ctx, alarm)
				e.redis.AddPendingAlarm(ctx, alarm.ID, nextTrigger)
				e.redis.AddChannelAlarm(ctx, alarm.Channel, alarm.ID)
			}
			log.Printf("[AlarmEngine] Loaded alarm %s (%s) next trigger at %d", alarm.ID, alarm.Type, nextTrigger)
		}
	}
}

// calcNextTrigger calculates the next trigger timestamp for an alarm.
// Returns 0 if the alarm should not be scheduled (e.g., expired single-use).
func (e *Engine) calcNextTrigger(alarm *models.Alarm) int64 {
	now := time.Now()

	switch alarm.TriggerMode {
	case models.AlarmTriggerAtTime:
		// Parse "HH:MM" format
		if len(alarm.TriggerTime) < 5 {
			return 0
		}
		var hour, min int
		fmt.Sscanf(alarm.TriggerTime, "%d:%d", &hour, &min)

		// Build today's trigger time
		trigger := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, now.Location())

		// If trigger time has passed today
		if trigger.Before(now) || trigger.Equal(now) {
			if alarm.Repeat == models.AlarmRepeatOnce {
				// Single-use alarm — if already passed, skip
				if alarm.LastTriggeredAt > 0 {
					return 0 // already triggered once
				}
				// It's passed today, so if it was never triggered, it's a missed alarm
				// We still schedule it for immediate trigger (backward compatibility)
				return now.Unix()
			}
			// For repeat alarms, schedule next occurrence
			trigger = trigger.Add(24 * time.Hour)
		}

		// For weekday/custom repeat, find the next valid day
		if alarm.Repeat == models.AlarmRepeatWeekday || alarm.Repeat == models.AlarmRepeatCustom {
			trigger = e.nextValidDay(trigger, alarm)
		}

		return trigger.Unix()

	case models.AlarmTriggerCountdown:
		// Countdown from creation time
		created := time.Unix(alarm.CreatedAt, 0)
		trigger := created.Add(time.Duration(alarm.CountdownMinutes) * time.Minute)

		if trigger.Before(now) {
			if alarm.Repeat == models.AlarmRepeatOnce {
				return 0 // expired
			}
			// For daily repeat countdown, schedule next occurrence from now
			trigger = now.Add(time.Duration(alarm.CountdownMinutes) * time.Minute)
		}
		return trigger.Unix()

	case models.AlarmTriggerSongCount:
		// Song-count based triggers are handled differently — they fire when N songs have played
		// We don't schedule them here; they're checked during song transitions
		return 0

	default:
		return 0
	}
}

// nextValidDay finds the next date that matches the repeat rules.
func (e *Engine) nextValidDay(trigger time.Time, alarm *models.Alarm) time.Time {
	// Try up to 14 days ahead to find a valid day
	for i := 0; i < 14; i++ {
		weekday := int(trigger.Weekday()) // 0=Sunday, 1=Monday, ...

		switch alarm.Repeat {
		case models.AlarmRepeatWeekday:
			if weekday >= 1 && weekday <= 5 {
				return trigger
			}
		case models.AlarmRepeatCustom:
			for _, d := range alarm.RepeatDays {
				if d == weekday {
					return trigger
				}
			}
		}
		trigger = trigger.Add(24 * time.Hour)
	}
	return trigger
}

// pollLoop runs every second to check for due alarms.
func (e *Engine) pollLoop() {
	defer e.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.processDueAlarms()
		}
	}
}

// processDueAlarms checks and executes due alarms.
func (e *Engine) processDueAlarms() {
	if e.redis == nil {
		return
	}

	ctx := context.Background()
	now := time.Now().Unix()

	// Get and remove due alarm IDs atomically
	alarmIDs, err := e.redis.RemoveDueAlarms(ctx, now, 100)
	if err != nil {
		log.Printf("[AlarmEngine] Failed to get due alarms: %v", err)
		return
	}

	for _, alarmID := range alarmIDs {
		e.executeAlarm(ctx, alarmID, now)
	}
}

// executeAlarm triggers an alarm and handles post-trigger logic (repeat scheduling, active stop tracking).
func (e *Engine) executeAlarm(ctx context.Context, alarmID string, now int64) {
	// Load alarm from Redis
	alarm, err := e.redis.GetAlarmHash(ctx, alarmID)
	if err != nil || alarm == nil {
		log.Printf("[AlarmEngine] Alarm %s not found in Redis, skipping", alarmID)
		return
	}

	if !alarm.Enabled {
		log.Printf("[AlarmEngine] Alarm %s is disabled, skipping", alarmID)
		return
	}

	// Get the channel
	ch := e.channelMgr.GetChannel(alarm.Channel)
	if ch == nil {
		log.Printf("[AlarmEngine] Channel %s not found for alarm %s, skipping", alarm.Channel, alarmID)
		// Mark as missed
		alarm.LastTriggeredAt = now
		e.redis.SetAlarmHash(ctx, alarm)
		return
	}

	// Update last triggered time
	alarm.LastTriggeredAt = now

	switch alarm.Type {
	case models.AlarmTypeStart:
		// Execute the alarm start callback
		if e.OnAlarmStart != nil {
			e.OnAlarmStart(alarm, ch)
		}

		// If there's an auto-stop configured, create the stop timer
		if alarm.AutoStopMode != "" && alarm.AutoStopMode != models.AlarmAutoStopNoLimit {
			stopAlarmID := e.createAutoStop(alarm, now)
			if stopAlarmID != "" {
				e.mu.Lock()
				e.activeStops[alarmID] = stopAlarmID
				e.mu.Unlock()
				e.redis.SetActiveStop(ctx, alarmID, stopAlarmID)
			}
		}

	case models.AlarmTypeSleepTimer:
		// Execute the sleep timer callback
		if e.OnSleepTimer != nil {
			e.OnSleepTimer(alarm, ch)
		}
	}

	// Handle repeat scheduling
	if alarm.Repeat != models.AlarmRepeatOnce {
		nextTrigger := e.calcNextTrigger(alarm)
		if nextTrigger > now {
			e.redis.AddPendingAlarm(ctx, alarmID, nextTrigger)
			log.Printf("[AlarmEngine] Rescheduled alarm %s for %d", alarmID, nextTrigger)
		}
	}

	// Save updated alarm (with LastTriggeredAt)
	e.redis.SetAlarmHash(ctx, alarm)
	if e.db != nil {
		e.db.UpdateAlarm(alarm)
	}

	log.Printf("[AlarmEngine] Executed alarm %s (%s) on channel %s", alarmID, alarm.Type, alarm.Channel)

	// Broadcast notification
	e.broadcastTriggered(alarm)
}

// createAutoStop creates a stop timer based on the alarm's auto-stop configuration.
// For play_time: creates a sleep_timer alarm scheduled N minutes in the future.
// For song_count: sets up a remaining-songs counter in Redis.
// Returns the stop alarm ID (for play_time) or empty string (for song_count, handled differently).
func (e *Engine) createAutoStop(alarm *models.Alarm, now int64) string {
	if alarm.AutoStopMode == models.AlarmAutoStopNoLimit || alarm.AutoStopValue <= 0 {
		return ""
	}

	switch alarm.AutoStopMode {
	case models.AlarmAutoStopPlayTime:
		// Stop after N minutes — create a real sleep_timer alarm
		var stopTimestamp int64
		stopTimestamp = now + int64(alarm.AutoStopValue)*60

		// Create a sleep timer alarm
		stopAlarm := &models.Alarm{
			ID:               fmt.Sprintf("auto-stop-%s", alarm.ID),
			Channel:          alarm.Channel,
			Type:             models.AlarmTypeSleepTimer,
			Enabled:          true,
			TriggerMode:      models.AlarmTriggerAtTime,
			TriggerTime:      time.Unix(stopTimestamp, 0).Format("15:04"),
			StopAction:       models.AlarmStopActionStop,
			FadeOutSeconds:   0,
			Repeat:           models.AlarmRepeatOnce,
			ConflictStrategy: models.AlarmConflictQueue,
			CreatedAt:        now,
		}

		// Persist and schedule
		ctx := context.Background()
		if e.redis != nil {
			e.redis.SetAlarmHash(ctx, stopAlarm)
			e.redis.AddPendingAlarm(ctx, stopAlarm.ID, stopTimestamp)
			e.redis.AddChannelAlarm(ctx, stopAlarm.Channel, stopAlarm.ID)
		}
		if e.db != nil {
			e.db.CreateAlarm(stopAlarm)
		}

		log.Printf("[AlarmEngine] Created auto-stop %s for alarm %s at %d", stopAlarm.ID, alarm.ID, stopTimestamp)
		return stopAlarm.ID

	case models.AlarmAutoStopSongCount:
		// Stop after N songs — store remaining count in Redis
		ctx := context.Background()
		if e.redis != nil {
			e.redis.SetRemainingSongs(ctx, alarm.ID, alarm.AutoStopValue)
			e.redis.AddActiveSongStop(ctx, alarm.Channel, alarm.ID)
		}
		log.Printf("[AlarmEngine] Set song-count auto-stop for alarm %s: stop after %d songs", alarm.ID, alarm.AutoStopValue)
		return ""
	}

	return ""
}

// CancelAutoStop cancels the associated auto-stop for an alarm start.
func (e *Engine) CancelAutoStop(alarmStartID string) {
	e.mu.Lock()
	stopID, ok := e.activeStops[alarmStartID]
	delete(e.activeStops, alarmStartID)
	e.mu.Unlock()

	if !ok || stopID == "" {
		return
	}

	ctx := context.Background()
	if e.redis != nil {
		e.redis.RemovePendingAlarm(ctx, stopID)
		e.redis.DeleteAlarmHash(ctx, stopID)
		e.redis.DeleteActiveStop(ctx, alarmStartID)
	}
	if e.db != nil {
		e.db.DeleteAlarm(stopID)
	}
	log.Printf("[AlarmEngine] Cancelled auto-stop %s for alarm %s", stopID, alarmStartID)
}

// ScheduleAlarm schedules a new alarm in Redis and DB.
func (e *Engine) ScheduleAlarm(alarm *models.Alarm) error {
	ctx := context.Background()

	// Ensure ID
	if alarm.ID == "" {
		alarm.ID = fmt.Sprintf("alarm-%d", time.Now().UnixNano())
	}
	alarm.CreatedAt = time.Now().Unix()
	alarm.UpdatedAt = time.Now().Unix()

	// Persist to Redis
	if e.redis != nil {
		if err := e.redis.SetAlarmHash(ctx, alarm); err != nil {
			return fmt.Errorf("failed to save alarm to Redis: %w", err)
		}
		if err := e.redis.AddChannelAlarm(ctx, alarm.Channel, alarm.ID); err != nil {
			return fmt.Errorf("failed to associate alarm with channel: %w", err)
		}
	}

	// Persist to DB
	if e.db != nil {
		if err := e.db.CreateAlarm(alarm); err != nil {
			return fmt.Errorf("failed to save alarm to DB: %w", err)
		}
	}

	// For sleep_timer with song_count trigger: set up songs counter instead of pending schedule
	if alarm.Type == models.AlarmTypeSleepTimer && alarm.TriggerMode == models.AlarmTriggerSongCount {
		if e.redis != nil && alarm.SongCount > 0 {
			e.redis.SetRemainingSongs(ctx, alarm.ID, alarm.SongCount)
			e.redis.AddActiveSongStop(ctx, alarm.Channel, alarm.ID)
			log.Printf("[AlarmEngine] Set song-count sleep timer %s: trigger after %d songs", alarm.ID, alarm.SongCount)
		}
		return nil
	}

	// Schedule next trigger for time-based alarms
	nextTrigger := e.calcNextTrigger(alarm)
	if nextTrigger > 0 && e.redis != nil {
		if err := e.redis.AddPendingAlarm(ctx, alarm.ID, nextTrigger); err != nil {
			return fmt.Errorf("failed to schedule alarm: %w", err)
		}
		log.Printf("[AlarmEngine] Scheduled alarm %s next trigger at %d", alarm.ID, nextTrigger)
	}

	return nil
}

// UpdateAlarm updates an existing alarm and reschedules it.
func (e *Engine) UpdateAlarm(alarm *models.Alarm) error {
	ctx := context.Background()
	now := time.Now()
	alarm.UpdatedAt = now.Unix()

	// Reset last triggered time so the alarm can fire again
	alarm.LastTriggeredAt = 0

	// For countdown/song_count triggers, reset CreatedAt so the timer restarts from now
	if alarm.TriggerMode == models.AlarmTriggerCountdown || alarm.TriggerMode == models.AlarmTriggerSongCount {
		alarm.CreatedAt = now.Unix()
	}

	// Remove old pending entry and song-count tracking
	if e.redis != nil {
		e.redis.RemovePendingAlarm(ctx, alarm.ID)
		e.redis.DeleteRemainingSongs(ctx, alarm.ID)
		// Get channel for cleanup (if we had it stored)
		oldAlarm, _ := e.redis.GetAlarmHash(ctx, alarm.ID)
		if oldAlarm != nil {
			e.redis.RemoveActiveSongStop(ctx, oldAlarm.Channel, alarm.ID)
		}
	}

	// Persist
	if e.redis != nil {
		if err := e.redis.SetAlarmHash(ctx, alarm); err != nil {
			return err
		}
	}

	if e.db != nil {
		if err := e.db.UpdateAlarm(alarm); err != nil {
			return err
		}
	}

	// Reschedule if enabled
	if alarm.Enabled {
		// For sleep_timer with song_count: set up counter instead of pending schedule
		if alarm.Type == models.AlarmTypeSleepTimer && alarm.TriggerMode == models.AlarmTriggerSongCount {
			if e.redis != nil && alarm.SongCount > 0 {
				e.redis.SetRemainingSongs(ctx, alarm.ID, alarm.SongCount)
				e.redis.AddActiveSongStop(ctx, alarm.Channel, alarm.ID)
				log.Printf("[AlarmEngine] Updated song-count sleep timer %s: trigger after %d songs", alarm.ID, alarm.SongCount)
			}
			return nil
		}

		nextTrigger := e.calcNextTrigger(alarm)
		if nextTrigger > 0 && e.redis != nil {
			e.redis.AddPendingAlarm(ctx, alarm.ID, nextTrigger)
		}
	}

	return nil
}

// DeleteAlarm removes an alarm from Redis and DB.
func (e *Engine) DeleteAlarm(alarmID string) error {
	ctx := context.Background()

	// Load alarm to get channel info for cleanup
	alarm, err := e.redis.GetAlarmHash(ctx, alarmID)
	if err != nil {
		return err
	}

	if e.redis != nil {
		e.redis.RemovePendingAlarm(ctx, alarmID)
		e.redis.DeleteAlarmHash(ctx, alarmID)
		// Clean up song count tracking if any
		e.redis.DeleteRemainingSongs(ctx, alarmID)
		if alarm != nil {
			e.redis.RemoveChannelAlarm(ctx, alarm.Channel, alarmID)
			e.redis.RemoveActiveSongStop(ctx, alarm.Channel, alarmID)
		}
	}

	if e.db != nil {
		if err := e.db.DeleteAlarm(alarmID); err != nil {
			return err
		}
	}

	return nil
}

// GetChannelAlarms returns all alarms for a channel.
func (e *Engine) GetChannelAlarms(channelName string) ([]*models.Alarm, error) {
	ctx := context.Background()

	if e.redis != nil {
		alarmIDs, err := e.redis.GetChannelAlarmIDs(ctx, channelName)
		if err != nil {
			return nil, err
		}

		alarms := make([]*models.Alarm, 0, len(alarmIDs))
		for _, id := range alarmIDs {
			alarm, err := e.redis.GetAlarmHash(ctx, id)
			if err != nil || alarm == nil {
				continue
			}
			alarms = append(alarms, alarm)
		}
		return alarms, nil
	}

	// Fallback to DB
	if e.db != nil {
		return e.db.ListChannelAlarms(channelName)
	}

	return nil, nil
}

// ResetAlarm resets a triggered alarm so it can fire again.
// Clears LastTriggeredAt, resets CreatedAt for countdown/song_count triggers,
// re-enables if disabled, and reschedules.
func (e *Engine) ResetAlarm(alarmID string) (*models.Alarm, error) {
	ctx := context.Background()

	alarm, err := e.redis.GetAlarmHash(ctx, alarmID)
	if err != nil || alarm == nil {
		return nil, fmt.Errorf("alarm not found: %s", alarmID)
	}

	now := time.Now().Unix()

	// Reset last triggered time
	alarm.LastTriggeredAt = 0

	// For countdown/song_count triggers, reset CreatedAt so the timer restarts from now
	if alarm.TriggerMode == models.AlarmTriggerCountdown || alarm.TriggerMode == models.AlarmTriggerSongCount {
		alarm.CreatedAt = now
	}

	// Ensure it's enabled
	alarm.Enabled = true
	alarm.UpdatedAt = now

	// Remove old pending / song-count tracking
	if e.redis != nil {
		e.redis.RemovePendingAlarm(ctx, alarm.ID)
		e.redis.DeleteRemainingSongs(ctx, alarm.ID)
		e.redis.RemoveActiveSongStop(ctx, alarm.Channel, alarm.ID)
	}

	// Persist
	if e.redis != nil {
		if err := e.redis.SetAlarmHash(ctx, alarm); err != nil {
			return nil, err
		}
	}

	if e.db != nil {
		if err := e.db.UpdateAlarm(alarm); err != nil {
			return nil, err
		}
	}

	// Reschedule
	if alarm.Type == models.AlarmTypeSleepTimer && alarm.TriggerMode == models.AlarmTriggerSongCount {
		if e.redis != nil && alarm.SongCount > 0 {
			e.redis.SetRemainingSongs(ctx, alarm.ID, alarm.SongCount)
			e.redis.AddActiveSongStop(ctx, alarm.Channel, alarm.ID)
			log.Printf("[AlarmEngine] Reset song-count sleep timer %s: trigger after %d songs", alarm.ID, alarm.SongCount)
		}
	} else {
		nextTrigger := e.calcNextTrigger(alarm)
		if nextTrigger > 0 && e.redis != nil {
			e.redis.AddPendingAlarm(ctx, alarm.ID, nextTrigger)
			log.Printf("[AlarmEngine] Reset and rescheduled alarm %s next trigger at %d", alarm.ID, nextTrigger)
		}
	}

	log.Printf("[AlarmEngine] Reset alarm %s (%s) on channel %s", alarm.ID, alarm.Type, alarm.Channel)
	return alarm, nil
}

// ToggleAlarm enables or disables an alarm.
func (e *Engine) ToggleAlarm(alarmID string) (*models.Alarm, error) {
	ctx := context.Background()

	alarm, err := e.redis.GetAlarmHash(ctx, alarmID)
	if err != nil || alarm == nil {
		return nil, fmt.Errorf("alarm not found: %s", alarmID)
	}

	alarm.Enabled = !alarm.Enabled
	alarm.UpdatedAt = time.Now().Unix()

	// Update in Redis
	if e.redis != nil {
		e.redis.SetAlarmHash(ctx, alarm)
	}

	// Update in DB
	if e.db != nil {
		e.db.UpdateAlarm(alarm)
	}

	// Reschedule or remove from pending
	if alarm.Enabled {
		nextTrigger := e.calcNextTrigger(alarm)
		if nextTrigger > 0 && e.redis != nil {
			e.redis.AddPendingAlarm(ctx, alarmID, nextTrigger)
		}
	} else {
		if e.redis != nil {
			e.redis.RemovePendingAlarm(ctx, alarmID)
		}
	}

	return alarm, nil
}

// GetActiveTimers returns the list of active timers (countdowns) for display.
func (e *Engine) GetActiveTimers(channelName string) ([]map[string]interface{}, error) {
	now := time.Now().Unix()

	alarms, err := e.GetChannelAlarms(channelName)
	if err != nil {
		return nil, err
	}

	var timers []map[string]interface{}
	for _, alarm := range alarms {
		if !alarm.Enabled {
			continue
		}

		// For countdown-based alarms, compute remaining time
		if alarm.TriggerMode == models.AlarmTriggerCountdown {
			created := alarm.CreatedAt
			triggerTime := created + int64(alarm.CountdownMinutes)*60
			remaining := triggerTime - now
			if remaining > 0 {
				timers = append(timers, map[string]interface{}{
					"alarm_id":         alarm.ID,
					"type":             alarm.Type,
					"remaining_seconds": remaining,
				})
			}
		} else if alarm.TriggerMode == models.AlarmTriggerAtTime && alarm.LastTriggeredAt == 0 {
			// For at_time, check if it's pending in the sorted set
			// The pending set gives us the next trigger time
			// Since we removed it from the set when processing, check if it has a repeat or not yet triggered
			if alarm.Repeat != models.AlarmRepeatOnce || alarm.LastTriggeredAt == 0 {
				// Parse time and compute remaining
				var hour, min int
				fmt.Sscanf(alarm.TriggerTime, "%d:%d", &hour, &min)
				trigger := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), hour, min, 0, 0, time.Now().Location())
				if trigger.Before(time.Now()) {
					if alarm.Repeat != models.AlarmRepeatOnce {
						trigger = trigger.Add(24 * time.Hour)
					} else {
						continue
					}
				}
				remaining := trigger.Unix() - now
				if remaining > 0 {
					timers = append(timers, map[string]interface{}{
						"alarm_id":          alarm.ID,
						"type":              alarm.Type,
						"remaining_seconds": remaining,
						"trigger_time":      alarm.TriggerTime,
					})
				}
			}
		}
	}

	return timers, nil
}

// broadcastTriggered sends a WebSocket notification about a triggered alarm.
func (e *Engine) broadcastTriggered(alarm *models.Alarm) {
	ch := e.channelMgr.GetChannel(alarm.Channel)
	if ch == nil {
		return
	}

	var action string
	var message string

	switch alarm.Type {
	case models.AlarmTypeStart:
		action = models.ActionAlarmTriggered
		message = "闹钟触发 - 开始播放"
	case models.AlarmTypeSleepTimer:
		action = models.ActionSleepTimerTriggered
		message = "定时关闭触发 - 停止播放"
	}

	notification := map[string]interface{}{
		"type":   "notification",
		"action": action,
		"payload": map[string]interface{}{
			"alarm_id": alarm.ID,
			"type":     alarm.Type,
			"channel":  alarm.Channel,
			"message":  message,
		},
	}

	// Broadcast to channel controls
	ch.BroadcastJSON(notification)

	log.Printf("[AlarmEngine] Broadcasted %s notification for alarm %s", action, alarm.ID)
}

// HandleSongFinished is called when a song finishes playing, to check song-count based auto-stops
// and song-count based sleep timers.
// Returns true if playback was stopped (caller should not advance to next song).
func (e *Engine) HandleSongFinished(ch *channel.Channel) bool {
	if e.redis == nil {
		return false
	}

	ctx := context.Background()

	// Get all active song-count alarms for this channel
	alarmIDs, err := e.redis.GetActiveSongStops(ctx, ch.Name)
	if err != nil || len(alarmIDs) == 0 {
		return false
	}

	for _, alarmID := range alarmIDs {
		remaining, err := e.redis.DecrementRemainingSongs(ctx, alarmID)
		if err != nil {
			continue
		}

		if remaining > 0 {
			log.Printf("[AlarmEngine] Song-count stop for alarm %s: %d songs remaining", alarmID, remaining)
			continue
		}

		// ── Count reached 0 ──
		// Remove tracking first
		e.redis.RemoveActiveSongStop(ctx, ch.Name, alarmID)
		e.redis.DeleteRemainingSongs(ctx, alarmID)

		// Load the alarm to determine its type and handle accordingly
		alarm, _ := e.redis.GetAlarmHash(ctx, alarmID)
		if alarm != nil {
			alarm.LastTriggeredAt = time.Now().Unix()
			e.redis.SetAlarmHash(ctx, alarm)
			if e.db != nil {
				e.db.UpdateAlarm(alarm)
			}
		}

		if alarm != nil && alarm.Type == models.AlarmTypeSleepTimer {
			// Manual sleep timer with song_count trigger:
			// call the sleep timer callback for proper stop handling
			log.Printf("[AlarmEngine] Song-count sleep timer triggered for alarm %s on channel %s", alarmID, ch.Name)
			if e.OnSleepTimer != nil {
				e.OnSleepTimer(alarm, ch)
			}
		} else {
			// Auto-stop (alarm_start's song-count): just pause
			log.Printf("[AlarmEngine] Song-count auto-stop reached for alarm %s on channel %s", alarmID, ch.Name)
			ch.BroadcastToPlayers(models.CmdPause, nil)
			ch.BroadcastJSON(map[string]interface{}{
				"type":   models.MsgTypeStateUpdate,
				"action": models.ActionQueueRefresh,
			})
		}

		// Broadcast notification
		ch.BroadcastJSON(map[string]interface{}{
			"type":   "notification",
			"action": models.ActionSleepTimerTriggered,
			"payload": map[string]interface{}{
				"alarm_id": alarmID,
				"type":     models.AlarmTypeSleepTimer,
				"channel":  ch.Name,
				"message":  "歌曲计数停止触发",
			},
		})

		return true
	}

	return false
}
