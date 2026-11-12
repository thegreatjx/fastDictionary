package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// The settings page has two asynchronous results: a connection probe and a
// model list. Each is a small explicit state machine with the configuration
// snapshot it belongs to. A result whose source no longer matches is not
// "stale" in a fuzzy sense; it is simply not shown, and it cannot overwrite a
// newer result because the generation is checked before the write.

type aiProbePhase uint8

const (
	aiProbeIdle aiProbePhase = iota
	aiProbeRunning
	aiProbeSucceeded
	aiProbeFailed
)

type aiProbeState struct {
	phase  aiProbePhase
	source string
	msg    string
	cancel context.CancelFunc
	gen    requestToken
}

type aiModelsPhase uint8

const (
	aiModelsIdle aiModelsPhase = iota
	aiModelsLoading
	aiModelsReady
	aiModelsUnsupported
	aiModelsFailed
)

type aiModelsState struct {
	phase  aiModelsPhase
	source string
	models []string
	err    string
	at     time.Time
	gen    requestToken
}

// defaultMagpieModels keeps the old gateway's common routes selectable while
// the gateway is down. They are only used for Magpie: a direct provider must
// never inherit these names.
var defaultMagpieModels = []string{
	magpieDefaultModel,
	"group/auto-deepseek-v4-1-flash",
	"workbuddy/deepseek-v4.1-flash",
	"antigravity/gemini-3.8-flash",
	"workbuddy/glm-5.3-flash",
	"workbuddy/kimi-k3-1",
	"minimax-code/MiniMax-M2.7-highspeed",
	"codex/gpt-5.5",
}

var fetchAvailableModelsFunc = FetchAvailableModels

// aiProbeFunc is a variable so the probe's stale-result rules can be tested
// against a function that blocks instead of a live gateway.
var aiProbeFunc = StreamAI

// testAIConnection probes the current service with a minimal request. The
// snapshot is captured before the goroutine starts; if the user edits the
// service while it runs, the result belongs to the old source and is dropped.
func (a *app) testAIConnection() {
	cfg := a.store.Get()
	source := aiProbeSource(cfg)
	if strings.TrimSpace(cfg.AIModel) == "" {
		a.mu.Lock()
		if a.aiProbe.cancel != nil {
			a.aiProbe.cancel()
		}
		a.aiProbe.gen++
		gen := a.aiProbe.gen
		a.aiProbe = aiProbeState{phase: aiProbeFailed, source: source, msg: "请先填写模型 ID。", gen: gen}
		a.mu.Unlock()
		a.settingsRefresh()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	a.mu.Lock()
	if a.aiProbe.cancel != nil {
		a.aiProbe.cancel()
	}
	a.aiProbe.gen++
	gen := a.aiProbe.gen
	a.aiProbe = aiProbeState{
		phase:  aiProbeRunning,
		source: source,
		msg:    "测试中…",
		cancel: cancel,
		gen:    gen,
	}
	a.mu.Unlock()
	a.settingsRefresh()

	go func() {
		defer recoverLog("ai_settings.go:testAIConnection")
		defer cancel()
		started := time.Now()
		out, err := aiProbeFunc(ctx, cfg.AIConn(cfg.AIModel),
			"You are a test probe. Reply with 'pong' only.", "ping",
			cfg.AIReasoningEffortFor(cfg.AIModel), nil)
		took := time.Since(started)

		a.mu.Lock()
		if a.aiProbe.gen != gen || a.aiProbe.source != source {
			a.mu.Unlock()
			return
		}
		a.aiProbe.cancel = nil
		if err != nil {
			a.aiProbe.phase = aiProbeFailed
			a.aiProbe.msg = "失败：" + aiErrorDetail(err)
		} else {
			a.aiProbe.phase = aiProbeSucceeded
			a.aiProbe.msg = fmt.Sprintf("正常 · %s · %s · 返回 %s",
				cfg.AIModel, took.Round(time.Millisecond), truncateFor(out, 20))
		}
		msg := a.aiProbe.msg
		a.mu.Unlock()

		a.settingsRefresh()
		log.Printf("fastDictionary: AI probe: %s", msg)
	}()
}

// aiProbeStatus returns the probe state only when it belongs to cfg. A copy is
// returned so the caller never holds the lock while rendering.
func (a *app) aiProbeStatus(cfg Config) aiProbeState {
	source := aiProbeSource(cfg)
	a.mu.Lock()
	state := a.aiProbe
	a.mu.Unlock()
	if state.source != source {
		return aiProbeState{}
	}
	state.cancel = nil
	return state
}

// loadAIModels fetches on settings open for Magpie only. A direct provider is
// fetched when the user asks: opening a settings page should not spend a
// request against an account the user has not finished configuring.
func (a *app) loadAIModels() {
	if cfg := a.store.Get(); !cfg.IsMagpie() {
		return
	}
	a.fetchAIModels(false)
}

func (a *app) refreshAIModels() {
	a.fetchAIModels(true)
}

func (a *app) fetchAIModels(force bool) {
	cfg := a.store.Get()
	source := aiEndpointSource(cfg)

	a.mu.Lock()
	if a.aiModels.phase == aiModelsLoading && a.aiModels.source == source {
		a.mu.Unlock()
		return
	}
	if !force && a.aiModels.phase == aiModelsReady &&
		a.aiModels.source == source && time.Since(a.aiModels.at) < 5*time.Minute {
		a.mu.Unlock()
		return
	}
	a.aiModels.gen++
	gen := a.aiModels.gen
	a.aiModels = aiModelsState{phase: aiModelsLoading, source: source, gen: gen}
	a.mu.Unlock()
	a.settingsRefresh()

	go func() {
		defer recoverLog("ai_settings.go:fetchAIModels")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		models, err := fetchAvailableModelsFunc(ctx, cfg.AIConn(cfg.AIModel))

		a.mu.Lock()
		if a.aiModels.gen != gen || a.aiModels.source != source {
			a.mu.Unlock()
			return
		}
		switch {
		case errors.Is(err, errAIModelsUnsupported):
			a.aiModels = aiModelsState{phase: aiModelsUnsupported, source: source, gen: gen, at: time.Now()}
		case err != nil:
			a.aiModels = aiModelsState{phase: aiModelsFailed, source: source, err: aiErrorDetail(err), gen: gen, at: time.Now()}
		default:
			a.aiModels = aiModelsState{phase: aiModelsReady, source: source, models: models, gen: gen, at: time.Now()}
		}
		a.mu.Unlock()
		a.settingsRefresh()
	}()
}

func (a *app) aiModelsStatus(cfg Config) aiModelsState {
	source := aiEndpointSource(cfg)
	a.mu.Lock()
	state := a.aiModels
	a.mu.Unlock()
	if state.source != source {
		return aiModelsState{}
	}
	return state
}

// getAIModels is the list the model selector shows. A direct provider without
// a successful fetch gets only its current model; the Magpie fallback names
// would be wrong there and are exactly the kind of hidden default this change
// is removing.
func (a *app) getAIModels(cfg Config) []string {
	return aiModelsForState(cfg, a.aiModelsStatus(cfg))
}

// aiModelsForState is the one place that turns a model state into the list the
// selector shows. The settings page passes the same state it renders, so the
// select and its note cannot disagree about whether a fetch succeeded.
func aiModelsForState(cfg Config, state aiModelsState) []string {
	if state.phase == aiModelsReady {
		return append([]string(nil), state.models...)
	}
	if cfg.IsMagpie() {
		return append([]string(nil), defaultMagpieModels...)
	}
	if model := strings.TrimSpace(cfg.AIModel); model != "" {
		return []string{model}
	}
	return nil
}

// resetAIStatus cancels a running probe and forgets both results. It is used
// when the service changes, where the old source is no longer meaningful.
func (a *app) resetAIStatus() {
	a.mu.Lock()
	if a.aiProbe.cancel != nil {
		a.aiProbe.cancel()
	}
	// Bump both generations before clearing. Keeping the counter monotonic
	// means an old goroutine cannot match the zero state by accident.
	a.aiProbe.gen++
	probeGen := a.aiProbe.gen
	a.aiProbe = aiProbeState{gen: probeGen}
	a.aiModels.gen++
	modelsGen := a.aiModels.gen
	a.aiModels = aiModelsState{gen: modelsGen}
	a.mu.Unlock()
}
