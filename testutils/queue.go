package testutils

import (
	"context"
	"orchdio/blueprint"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/hibiken/asynq"
	"github.com/jmoiron/sqlx"
)

// QueueService defines the interface for queue operations
type QueueService interface {
	// Task related operations
	NewTask(taskType, queue string, retry int, payload []byte) (*asynq.Task, error)
	EnqueueTask(task *asynq.Task, queue, taskId string, processIn time.Duration) error
	RunTask(pattern string, handler func(context.Context, *asynq.Task) error)

	// Playlist related operations
	NewPlaylistQueue(entityID string, payload *blueprint.LinkInfo) (*asynq.Task, error)
	PlaylistTaskHandler(ctx context.Context, task *asynq.Task) error
	PlaylistHandler(uid, shorturl string, info *blueprint.LinkInfo, appId string) error

	// Email related operations
	SendEmail(emailData *blueprint.EmailTaskData) error
	SendEmailHandler(ctx context.Context, task *asynq.Task) error
}

type MockQueue struct {
	mockNewTask          func(taskType, queue string, retry int, payload []byte) (*asynq.Task, error)
	mockEnqueueTask      func(task *asynq.Task, queue, taskId string, processIn time.Duration) error
	mockRunTask          func(pattern string, handler func(context.Context, *asynq.Task) error)
	mockNewPlaylistQueue func(entityID string, payload *blueprint.LinkInfo) (*asynq.Task, error)
	mockPlaylistHandler  func(uid, shorturl string, info *blueprint.LinkInfo, appId string) error
	mockSendEmail        func(emailData *blueprint.EmailTaskData) error
}

func (m *MockQueue) NewTask(taskType, queue string, retry int, payload []byte) (*asynq.Task, error) {
	if m.mockNewTask != nil {
		return m.mockNewTask(taskType, queue, retry, payload)
	}
	return asynq.NewTask(taskType, payload), nil
}

func (m *MockQueue) EnqueueTask(task *asynq.Task, queue, taskId string, processIn time.Duration) error {
	if m.mockEnqueueTask != nil {
		return m.mockEnqueueTask(task, queue, taskId, processIn)
	}
	return nil
}

func (m *MockQueue) RunTask(pattern string, handler func(context.Context, *asynq.Task) error) {
	if m.mockRunTask != nil {
		m.mockRunTask(pattern, handler)
	}
}

func (m *MockQueue) NewPlaylistQueue(entityID string, payload *blueprint.LinkInfo) (*asynq.Task, error) {
	if m.mockNewPlaylistQueue != nil {
		return m.mockNewPlaylistQueue(entityID, payload)
	}
	return asynq.NewTask(entityID, []byte{}), nil
}

func (m *MockQueue) PlaylistTaskHandler(ctx context.Context, task *asynq.Task) error {
	return nil
}

func (m *MockQueue) PlaylistHandler(uid, shorturl string, info *blueprint.LinkInfo, appId string) error {
	if m.mockPlaylistHandler != nil {
		return m.mockPlaylistHandler(uid, shorturl, info, appId)
	}
	return nil
}

func (m *MockQueue) SendEmail(emailData *blueprint.EmailTaskData) error {
	if m.mockSendEmail != nil {
		return m.mockSendEmail(emailData)
	}
	return nil
}

func (m *MockQueue) SendEmailHandler(ctx context.Context, task *asynq.Task) error {
	return nil
}

// Example test helper functions
func NewMockQueue(asynqClient *asynq.Client, db *sqlx.DB, red *redis.Client, router *asynq.ServeMux) *MockQueue {
	return &MockQueue{}
}

func (m *MockQueue) WithMockNewTask(fn func(taskType, queue string, retry int, payload []byte) (*asynq.Task, error)) *MockQueue {
	m.mockNewTask = fn
	return m
}

func (m *MockQueue) WithMockEnqueueTask(fn func(task *asynq.Task, queue, taskId string, processIn time.Duration) error) *MockQueue {
	m.mockEnqueueTask = fn
	return m
}

func (m *MockQueue) WithMockPlaylistHandler(fn func(uid, shorturl string, info *blueprint.LinkInfo, appId string) error) *MockQueue {
	m.mockPlaylistHandler = fn
	return m
}
