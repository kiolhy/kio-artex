package llmrec

// emptyretry.go —— 空响应重试（来源：二开 adeljck d562415）。
//
// 现象：某些 provider / 网关（尤其开了 thinking 或挂了中转）会返回一个"合法但空"
// 的响应——HTTP 200、正常 message_start/message_stop、usage 也记了，但既没有正文
// 也没有工具调用。上层 agent 拿到的是一个空回合，只能靠 engine 的 emptyTurnNudge
// 反复催促，白烧一轮 token 和时间。
//
// 做法：在 llmrec 这一层（所有 provider / 所有 format 都必经）检测"整条流里一个内容
// 事件都没有"，就地把 max_tokens 翻倍重试一次。之所以放在这层而不是每个 provider 各
// 写一遍，是因为这里能同时覆盖 anthropic / openai / openai-responses 三种 format，
// 以及流式与非流式两条路径。
//
// 关键约束：正常响应的"边收边吐"不能被打断。因此只在收到第一个内容事件之前做缓冲
// （缓冲里只有 message_start/message_delta 这类元数据），一旦出现内容就把缓冲一次性
// flush 出去，之后完全按原样透传。空响应时缓冲直接丢弃，调用方不会看到任何重复事件。

import (
	"os"
	"strings"

	"github.com/Autumn-27/norma/llm"
)

// emptyRetryEnabled 报告是否启用空响应重试（默认开，ARTEX_LLM_EMPTY_RETRY=0 可关）。
func emptyRetryEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ARTEX_LLM_EMPTY_RETRY"))) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// isContentEvent 判断一个流事件是否代表"模型真的产出了东西"。
//
// 只有文本、思考、工具调用算内容；message_start / message_delta / message_stop
// 只承载元数据与用量，空响应也会照常出现，不能用来判定非空。
func isContentEvent(ev llm.StreamEvent) bool {
	switch ev.Type {
	case llm.SETextDelta:
		return strings.TrimSpace(ev.Text) != ""
	case llm.SEThinkingDelta:
		return strings.TrimSpace(ev.Text) != ""
	case llm.SEThinkingSignature:
		return strings.TrimSpace(ev.Text) != ""
	case llm.SEToolUseStart:
		return true
	case llm.SEToolInputJSON:
		return strings.TrimSpace(ev.Text) != ""
	}
	return false
}

// doubleMaxTokens 返回把输出上限翻倍后的请求（上限为 0 表示"不发上限"，
// 由服务端默认值决定——那种情况翻倍没有意义，原样返回）。
func doubleMaxTokens(req llm.CompletionRequest) (llm.CompletionRequest, bool) {
	if req.MaxTokens <= 0 {
		return req, false
	}
	out := req
	out.MaxTokens = req.MaxTokens * 2
	return out, true
}

// messageHasContent 判断非流式响应是否为空（无正文、无思考、无工具调用）。
func messageHasContent(msg llm.Message) bool {
	for _, b := range msg.Content {
		switch b.Type {
		case llm.BlockText:
			if strings.TrimSpace(b.Text) != "" {
				return true
			}
		case llm.BlockThinking:
			if strings.TrimSpace(b.Thinking) != "" {
				return true
			}
		case llm.BlockToolUse:
			return true
		}
	}
	return false
}
