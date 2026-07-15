package redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-redis/redis/v8"
	"music-player/models"
)

type Client struct {
	rdb *redis.Client
}

func New(addr string) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &Client{rdb: rdb}, nil
}

func queueKey(channel string) string {
	return "channel:" + channel + ":queue"
}

func currentIndexKey(channel string) string {
	return "channel:" + channel + ":current_index"
}

func playerChannelKey(playerID string) string {
	return "player:" + playerID + ":channel"
}

func (c *Client) GetQueue(ctx context.Context, channel string) ([]models.Song, error) {
	data, err := c.rdb.LRange(ctx, queueKey(channel), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	songs := make([]models.Song, 0, len(data))
	for _, s := range data {
		var song models.Song
		if err := json.Unmarshal([]byte(s), &song); err != nil {
			continue
		}
		songs = append(songs, song)
	}
	return songs, nil
}

func (c *Client) AddToQueue(ctx context.Context, channel string, song models.Song) error {
	data, err := json.Marshal(song)
	if err != nil {
		return err
	}
	return c.rdb.RPush(ctx, queueKey(channel), data).Err()
}

func (c *Client) RemoveFromQueue(ctx context.Context, channel string, index int) error {
	// LSet the value at index to a tombstone marker, then LRem
	// Actually, Redis doesn't have a direct "remove at index" for lists.
	// We'll use Lua script for atomicity.
	script := `
		local key = KEYS[1]
		local idx = tonumber(ARGV[1])
		local len = redis.call("LLEN", key)
		if idx < 0 or idx >= len then
			return 0
		end
		local val = redis.call("LINDEX", key, idx)
		redis.call("LREM", key, 1, val)
		return 1
	`
	return c.rdb.Eval(ctx, script, []string{queueKey(channel)}, index).Err()
}

func (c *Client) ReorderQueue(ctx context.Context, channel string, from, to int) error {
	script := `
		local key = KEYS[1]
		local from = tonumber(ARGV[1])
		local to = tonumber(ARGV[2])
		local len = redis.call("LLEN", key)
		if from < 0 or from >= len or to < 0 or to >= len then
			return 0
		end
		local val = redis.call("LINDEX", key, from)
		redis.call("LREM", key, 1, val)
		redis.call("LINSERT", key, "BEFORE", redis.call("LINDEX", key, to), val)
		return 1
	`
	return c.rdb.Eval(ctx, script, []string{queueKey(channel)}, from, to).Err()
}

func (c *Client) ClearQueue(ctx context.Context, channel string) error {
	return c.rdb.Del(ctx, queueKey(channel), currentIndexKey(channel)).Err()
}

func (c *Client) GetCurrentIndex(ctx context.Context, channel string) (int, error) {
	idx, err := c.rdb.Get(ctx, currentIndexKey(channel)).Int()
	if err == redis.Nil {
		return 0, nil
	}
	return idx, err
}

func (c *Client) SetCurrentIndex(ctx context.Context, channel string, index int) error {
	return c.rdb.Set(ctx, currentIndexKey(channel), index, 0).Err()
}

func (c *Client) SetPlayerChannel(ctx context.Context, playerID, channel string) error {
	return c.rdb.Set(ctx, playerChannelKey(playerID), channel, 0).Err()
}

func (c *Client) GetPlayerChannel(ctx context.Context, playerID string) (string, error) {
	ch, err := c.rdb.Get(ctx, playerChannelKey(playerID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return ch, err
}

func (c *Client) DelPlayerChannel(ctx context.Context, playerID string) error {
	return c.rdb.Del(ctx, playerChannelKey(playerID)).Err()
}

// ─── Alarm Redis Operations ───────────────────────────────

func alarmHashKey(id string) string {
	return "alarm:" + id
}

func pendingAlarmsKey() string {
	return "alarms:pending"
}

func channelAlarmsKey(channel string) string {
	return "channel:" + channel + ":alarms"
}

func activeStopKey(alarmID string) string {
	return "alarm:" + alarmID + ":active_stop"
}

// SetAlarmHash stores an alarm definition as a Redis Hash.
func (c *Client) SetAlarmHash(ctx context.Context, alarm *models.Alarm) error {
	data, err := json.Marshal(alarm)
	if err != nil {
		return err
	}
	return c.rdb.HSet(ctx, alarmHashKey(alarm.ID), map[string]interface{}{
		"data":    string(data),
		"channel": alarm.Channel,
		"type":    alarm.Type,
		"enabled": alarm.Enabled,
	}).Err()
}

// GetAlarmHash retrieves an alarm definition from Redis hash.
func (c *Client) GetAlarmHash(ctx context.Context, id string) (*models.Alarm, error) {
	data, err := c.rdb.HGet(ctx, alarmHashKey(id), "data").Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var alarm models.Alarm
	if err := json.Unmarshal([]byte(data), &alarm); err != nil {
		return nil, err
	}
	return &alarm, nil
}

// DeleteAlarmHash removes an alarm definition from Redis.
func (c *Client) DeleteAlarmHash(ctx context.Context, id string) error {
	return c.rdb.Del(ctx, alarmHashKey(id)).Err()
}

// AddPendingAlarm adds an alarm to the pending sorted set (score = trigger timestamp).
func (c *Client) AddPendingAlarm(ctx context.Context, alarmID string, triggerTimestamp int64) error {
	return c.rdb.ZAdd(ctx, pendingAlarmsKey(), &redis.Z{
		Score:  float64(triggerTimestamp),
		Member: alarmID,
	}).Err()
}

// RemovePendingAlarm removes an alarm from the pending sorted set.
func (c *Client) RemovePendingAlarm(ctx context.Context, alarmID string) error {
	return c.rdb.ZRem(ctx, pendingAlarmsKey(), alarmID).Err()
}

// GetDueAlarms returns all alarm IDs with score <= now, up to limit.
func (c *Client) GetDueAlarms(ctx context.Context, now int64, limit int64) ([]string, error) {
	return c.rdb.ZRangeByScore(ctx, pendingAlarmsKey(), &redis.ZRangeBy{
		Min:    "-inf",
		Max:    fmt.Sprintf("%d", now),
		Offset: 0,
		Count:  limit,
	}).Result()
}

// RemoveDueAlarms removes and returns up to `limit` alarm IDs with score <= now.
func (c *Client) RemoveDueAlarms(ctx context.Context, now int64, limit int64) ([]string, error) {
	// Use a Lua script for atomic pop
	script := `
		local key = KEYS[1]
		local max_score = ARGV[1]
		local limit = tonumber(ARGV[2])
		local members = redis.call("ZRANGEBYSCORE", key, "-inf", max_score, "LIMIT", 0, limit)
		if #members > 0 then
			redis.call("ZREM", key, unpack(members))
		end
		return members
	`
	result, err := c.rdb.Eval(ctx, script, []string{pendingAlarmsKey()}, fmt.Sprintf("%d", now), limit).Result()
	if err != nil {
		return nil, err
	}
	members, ok := result.([]interface{})
	if !ok {
		return nil, nil
	}
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i], _ = m.(string)
	}
	return ids, nil
}

// AddChannelAlarm associates an alarm with a channel.
func (c *Client) AddChannelAlarm(ctx context.Context, channel, alarmID string) error {
	return c.rdb.SAdd(ctx, channelAlarmsKey(channel), alarmID).Err()
}

// RemoveChannelAlarm removes the alarm-channel association.
func (c *Client) RemoveChannelAlarm(ctx context.Context, channel, alarmID string) error {
	return c.rdb.SRem(ctx, channelAlarmsKey(channel), alarmID).Err()
}

// GetChannelAlarmIDs returns all alarm IDs associated with a channel.
func (c *Client) GetChannelAlarmIDs(ctx context.Context, channel string) ([]string, error) {
	return c.rdb.SMembers(ctx, channelAlarmsKey(channel)).Result()
}

// DeleteChannelAlarmsKey removes the entire channel alarms set.
func (c *Client) DeleteChannelAlarmsKey(ctx context.Context, channel string) error {
	return c.rdb.Del(ctx, channelAlarmsKey(channel)).Err()
}

// SetActiveStop associates an auto-stop timer ID with an alarm start.
func (c *Client) SetActiveStop(ctx context.Context, alarmID, stopAlarmID string) error {
	return c.rdb.Set(ctx, activeStopKey(alarmID), stopAlarmID, 0).Err()
}

// GetActiveStop retrieves the associated auto-stop alarm ID.
func (c *Client) GetActiveStop(ctx context.Context, alarmID string) (string, error) {
	val, err := c.rdb.Get(ctx, activeStopKey(alarmID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// DeleteActiveStop removes the active stop association.
func (c *Client) DeleteActiveStop(ctx context.Context, alarmID string) error {
	return c.rdb.Del(ctx, activeStopKey(alarmID)).Err()
}

// ─── Song Count Auto-Stop ────────────────────────────────

func remainingSongsKey(alarmID string) string {
	return "alarm:" + alarmID + ":remaining_songs"
}

func activeSongStopSetKey(channel string) string {
	return "channel:" + channel + ":active_song_stops"
}

// SetRemainingSongs stores the number of songs left to play before auto-stop.
func (c *Client) SetRemainingSongs(ctx context.Context, alarmID string, count int) error {
	return c.rdb.Set(ctx, remainingSongsKey(alarmID), count, 0).Err()
}

// DecrementRemainingSongs atomically decrements and returns the remaining count.
func (c *Client) DecrementRemainingSongs(ctx context.Context, alarmID string) (int, error) {
	val, err := c.rdb.Decr(ctx, remainingSongsKey(alarmID)).Result()
	return int(val), err
}

// DeleteRemainingSongs removes the remaining songs counter.
func (c *Client) DeleteRemainingSongs(ctx context.Context, alarmID string) error {
	return c.rdb.Del(ctx, remainingSongsKey(alarmID)).Err()
}

// AddActiveSongStop registers an alarm as having an active song-count auto-stop.
func (c *Client) AddActiveSongStop(ctx context.Context, channel, alarmID string) error {
	return c.rdb.SAdd(ctx, activeSongStopSetKey(channel), alarmID).Err()
}

// RemoveActiveSongStop removes an alarm from the active song-count set.
func (c *Client) RemoveActiveSongStop(ctx context.Context, channel, alarmID string) error {
	return c.rdb.SRem(ctx, activeSongStopSetKey(channel), alarmID).Err()
}

// GetActiveSongStops returns all alarm IDs with active song-count auto-stops for a channel.
func (c *Client) GetActiveSongStops(ctx context.Context, channel string) ([]string, error) {
	return c.rdb.SMembers(ctx, activeSongStopSetKey(channel)).Result()
}

func (c *Client) Close() error {
	return c.rdb.Close()
}
