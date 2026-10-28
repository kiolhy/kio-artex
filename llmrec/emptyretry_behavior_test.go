package llmrec

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Autumn-27/norma/llm"
)

// noRecord 关掉正文录制（测试里没有数据库，record() 需要 *db.DB）。
func noRecord() bool { return false }

// fakeProvider 按调用次数返回预设的流。第 n 次调用用 attempts[n-1]。
type fakeProvider struct {
	attempts [][]llm.StreamEvent
	calls    int
	reqs     []llm.CompletionRequest
	// completeMsgs 用于非流式路径；不足时返回空消息。
	completeMsgs []llm.Message
}

func (f *fakeProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	f.reqs = append(f.reqs, req)
	idx := f.calls
	f.calls++
	return func(yield func(llm.StreamEvent, error) bool) {
		if idx >= len(f.attempts) {
			return
		}
		for _, ev := range f.attempts[idx] {
			if !yield(ev, nil) {
				return
			}
		}
	}
}

func (f *fakeProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	f.reqs = append(f.reqs, req)
	idx := f.calls
	f.calls++
	if idx < len(f.completeMsgs) {
		return f.completeMsgs[idx], "end_turn", llm.Usage{InputTokens: 10, OutputTokens: 5}, nil
	}
	return llm.Message{}, "end_turn", llm.Usage{}, nil
}

func collect(t *testing.T, seq iter.Seq2[llm.StreamEvent, error]) ([]llm.StreamEvent, error) {
	t.Helper()
	var out []llm.StreamEvent
	var gotErr error
	for ev, err := range seq {
		if err != nil {
			gotErr = err
			break
		}
		out = append(out, ev)
	}
	return out, gotErr
}

// 空响应（只有元数据）必须触发一次重试，且第二次的内容要完整送达调用方。
func TestStreamRetriesOnEmptyResponse(t *testing.T) {
	empty := []llm.StreamEvent{
		{Type: llm.SEMessageStart, Usage: llm.Usage{InputTokens: 100}},
		{Type: llm.SEMessageDelta, StopReason: "end_turn"},
		{Type: llm.SEMessageStop},
	}
	real := []llm.StreamEvent{
		{Type: llm.SEMessageStart, Usage: llm.Usage{InputTokens: 100}},
		{Type: llm.SETextDelta, Text: "hello"},
		{Type: llm.SEMessageDelta, StopReason: "end_turn", Usage: llm.Usage{OutputTokens: 7}},
	}
	inner := &fakeProvider{attempts: [][]llm.StreamEvent{empty, real}}
	rec := Wrap(inner, nil, "m", "p", "", "", noRecord)

	got, err := collect(t, rec.Stream(context.Background(), llm.CompletionRequest{MaxTokens: 1024}))
	if err != nil {
		t.Fatalf("未预期错误: %v", err)
	}
	if inner.calls != 2 {
		t.Fatalf("应重试一次（共 2 次调用），实际 %d", inner.calls)
	}
	if inner.reqs[1].MaxTokens != 2048 {
		t.Errorf("重试应把 max_tokens 翻倍，实际 %d", inner.reqs[1].MaxTokens)
	}
	// 第一次尝试的元数据不能重复出现在输出里：只有第二次的 message_start。
	starts := 0
	var text string
	for _, ev := range got {
		switch ev.Type {
		case llm.SEMessageStart:
			starts++
		case llm.SETextDelta:
			text += ev.Text
		}
	}
	if starts != 1 {
		t.Errorf("message_start 应只出现一次（空响应那轮被丢弃），实际 %d", starts)
	}
	if text != "hello" {
		t.Errorf("正文应为 hello，实际 %q", text)
	}
}

// 正常响应不得被打断：事件顺序与数量必须与底层完全一致。
func TestStreamPassesThroughNonEmptyResponse(t *testing.T) {
	events := []llm.StreamEvent{
		{Type: llm.SEMessageStart},
		{Type: llm.SEThinkingDelta, Text: "hmm"},
		{Type: llm.SETextDelta, Text: "a"},
		{Type: llm.SETextDelta, Text: "b"},
		{Type: llm.SEMessageStop},
	}
	inner := &fakeProvider{attempts: [][]llm.StreamEvent{events}}
	rec := Wrap(inner, nil, "m", "p", "", "", noRecord)

	got, err := collect(t, rec.Stream(context.Background(), llm.CompletionRequest{MaxTokens: 1024}))
	if err != nil {
		t.Fatalf("未预期错误: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("正常响应不该重试，实际调用 %d 次", inner.calls)
	}
	if len(got) != len(events) {
		t.Fatalf("事件数应为 %d，实际 %d", len(events), len(got))
	}
	for i := range events {
		if got[i].Type != events[i].Type || got[i].Text != events[i].Text {
			t.Errorf("第 %d 个事件不一致: %+v vs %+v", i, got[i], events[i])
		}
	}
}

// 工具调用也算内容：只有 tool_use 的响应绝不能重试（否则工具会被重复执行）。
func TestStreamToolUseIsContent(t *testing.T) {
	events := []llm.StreamEvent{
		{Type: llm.SEMessageStart},
		{Type: llm.SEToolUseStart, ToolID: "t1", ToolName: "Bash"},
		{Type: llm.SEToolInputJSON, Text: `{"command":"ls"}`},
		{Type: llm.SEMessageStop},
	}
	inner := &fakeProvider{attempts: [][]llm.StreamEvent{events}}
	rec := Wrap(inner, nil, "m", "p", "", "", noRecord)

	if _, err := collect(t, rec.Stream(context.Background(), llm.CompletionRequest{MaxTokens: 512})); err != nil {
		t.Fatalf("未预期错误: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("带工具调用的响应不该重试，实际调用 %d 次", inner.calls)
	}
}

// 没有输出上限可翻倍时不重试：翻倍无从谈起，重试只是白烧一次。
func TestStreamNoRetryWithoutMaxTokens(t *testing.T) {
	empty := []llm.StreamEvent{{Type: llm.SEMessageStart}, {Type: llm.SEMessageStop}}
	inner := &fakeProvider{attempts: [][]llm.StreamEvent{empty, empty}}
	rec := Wrap(inner, nil, "m", "p", "", "", noRecord)

	if _, err := collect(t, rec.Stream(context.Background(), llm.CompletionRequest{})); err != nil {
		t.Fatalf("未预期错误: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("无 max_tokens 时不该重试，实际 %d 次", inner.calls)
	}
}

// 开关关掉后必须退回原行为。
func TestStreamRetryDisabledByEnv(t *testing.T) {
	t.Setenv("ARTEX_LLM_EMPTY_RETRY", "0")
	empty := []llm.StreamEvent{{Type: llm.SEMessageStart}, {Type: llm.SEMessageStop}}
	inner := &fakeProvider{attempts: [][]llm.StreamEvent{empty, empty}}
	rec := Wrap(inner, nil, "m", "p", "", "", noRecord)

	if _, err := collect(t, rec.Stream(context.Background(), llm.CompletionRequest{MaxTokens: 256})); err != nil {
		t.Fatalf("未预期错误: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("开关关闭后不该重试，实际 %d 次", inner.calls)
	}
}

// 底层报错必须原样透出，且不重试。
func TestStreamErrorNotRetried(t *testing.T) {
	inner := &errProvider{err: errors.New("boom")}
	rec := Wrap(inner, nil, "m", "p", "", "", noRecord)
	_, err := collect(t, rec.Stream(context.Background(), llm.CompletionRequest{MaxTokens: 256}))
	if err == nil || err.Error() != "boom" {
		t.Fatalf("应透出底层错误，实际 %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("报错不该重试，实际 %d 次", inner.calls)
	}
}

type errProvider struct {
	err   error
	calls int
}

func (p *errProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	p.calls++
	return func(yield func(llm.StreamEvent, error) bool) {
		yield(llm.StreamEvent{}, p.err)
	}
}

func (p *errProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	p.calls++
	return llm.Message{}, "", llm.Usage{}, p.err
}

// 非流式路径同样要重试一次，且两次用量合并。
func TestCompleteRetriesOnEmptyMessage(t *testing.T) {
	inner := &fakeProvider{
		completeMsgs: []llm.Message{
			{}, // 第一次：空
			{Content: []llm.ContentBlock{{Type: llm.BlockText, Text: "done"}}}, // 第二次：有内容
		},
	}
	rec := Wrap(inner, nil, "m", "p", "", "", noRecord)

	msg, _, usage, err := rec.Complete(context.Background(), llm.CompletionRequest{MaxTokens: 512})
	if err != nil {
		t.Fatalf("未预期错误: %v", err)
	}
	if inner.calls != 2 {
		t.Fatalf("应重试一次，实际 %d 次", inner.calls)
	}
	if inner.reqs[1].MaxTokens != 1024 {
		t.Errorf("重试应翻倍 max_tokens，实际 %d", inner.reqs[1].MaxTokens)
	}
	if msg.Text() != "done" {
		t.Errorf("应拿到第二次的内容，实际 %q", msg.Text())
	}
	if usage.InputTokens != 20 || usage.OutputTokens != 10 {
		t.Errorf("两次调用的用量必须合并，实际 %+v", usage)
	}
}

func TestDoubleMaxTokens(t *testing.T) {
	if _, ok := doubleMaxTokens(llm.CompletionRequest{}); ok {
		t.Error("MaxTokens=0 时不该声称可以翻倍")
	}
	got, ok := doubleMaxTokens(llm.CompletionRequest{MaxTokens: 100})
	if !ok || got.MaxTokens != 200 {
		t.Errorf("翻倍结果不对: %+v ok=%v", got, ok)
	}
}
