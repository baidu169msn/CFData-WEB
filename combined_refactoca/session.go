package main

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

var globalRunningTasks int32
var globalTaskMutex sync.Mutex

func anyTaskRunning() bool {
	return atomic.LoadInt32(&globalRunningTasks) > 0
}

func safeGo(label string, session *appSession, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprintf("%s panic: %v", label, r)
				fmt.Printf("%s\n%s\n", msg, debug.Stack())
				recordProgramDebugError("panic", fmt.Sprintf("%s\n%s", msg, debug.Stack()))
				if session != nil {
					session.sendWSMessage("error", "内部错误: "+msg)
				}
			}
		}()
		fn()
	}()
}

// sendWSMessage 将任务事件交给 CLI 回调处理（历史命名，保留以减少改动）。
func (s *appSession) sendWSMessage(msgType string, data interface{}) {
	if s.emit == nil {
		return
	}
	if msgType == "error" || msgType == "github_upload_error" {
		recordProgramDebugError(msgType, data)
	} else if msgType == "log" || msgType == "github_upload_status" {
		recordDebugNotice(msgType, data)
	}
	s.emit(msgType, data)
}

func (s *appSession) runTaskSync(run taskStarter) error {
	ctx, cancel := context.WithCancel(context.Background())
	if !s.beginTask(cancel) {
		cancel()
		return context.Canceled
	}
	defer cancel()
	defer s.endTask()
	run(ctx, s)
	return nil
}

func (s *appSession) beginTask(cancel context.CancelFunc) bool {
	globalTaskMutex.Lock()
	defer globalTaskMutex.Unlock()
	s.taskMutex.Lock()
	defer s.taskMutex.Unlock()
	if anyTaskRunning() || s.isTaskRunning {
		return false
	}
	s.isTaskRunning = true
	s.taskCancel = cancel
	atomic.AddInt32(&globalRunningTasks, 1)
	return true
}

func (s *appSession) endTask() {
	globalTaskMutex.Lock()
	defer globalTaskMutex.Unlock()
	s.taskMutex.Lock()
	defer s.taskMutex.Unlock()
	if s.isTaskRunning {
		atomic.AddInt32(&globalRunningTasks, -1)
	}
	s.isTaskRunning = false
	s.taskCancel = nil
}
