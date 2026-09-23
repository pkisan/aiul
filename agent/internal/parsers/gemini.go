package parsers

import (
	"encoding/json"
	"strings"
)

// Gemini parses generativelanguage.googleapis.com, used by the Gemini CLI and the
// Google AI SDKs. The model name is in the path rather than the body:
//
//	/v1beta/models/gemini-2.5-pro:streamGenerateContent
type Gemini struct{}

func (Gemini) Name() string { return "gemini" }

func (Gemini) Handles(host, path string) bool {
	// The method is ":generateContent" or ":streamGenerateContent", so compare
	// case-insensitively rather than guessing the capital letter.
	return host == "generativelanguage.googleapis.com" &&
		strings.Contains(strings.ToLower(path), "generatecontent")
}

func (p Gemini) Parse(ex Exchange) (Result, error) {
	var res Result
	res.Tool = toolFromHeaders(ex.ReqHead)
	res.Model = modelFromGeminiPath(ex.Path)
	res.Streamed = strings.Contains(strings.ToLower(ex.Path), "streamgeneratecontent")

	var req struct {
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text             string          `json:"text"`
				FunctionResponse json.RawMessage `json:"functionResponse"`
			} `json:"parts"`
		} `json:"contents"`
		SystemInstruction struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"systemInstruction"`
	}
	if err := json.Unmarshal(ex.ReqBody, &req); err != nil {
		return res, err
	}

	for _, p := range req.SystemInstruction.Parts {
		res.System += p.Text
	}

	for _, c := range req.Contents {
		if c.Role != "" && c.Role != "user" {
			continue
		}
		var b strings.Builder
		for _, part := range c.Parts {
			b.WriteString(part.Text)
			if len(part.FunctionResponse) > 0 {
				res.Automated = true
			}
		}
		if b.Len() > 0 {
			res.Prompt = b.String()
		}
	}

	if len(ex.SSE) > 0 {
		res.Streamed = true
		res.Answer, res.PromptTokens, res.ResponseTokens = p.reassemble(ex.SSE)
		return res, nil
	}

	p.parseWholeResponse(ex.RespBody, &res)
	return res, nil
}

// geminiResponse is the same shape whether it arrives whole or as one streamed
// chunk, which is why both paths decode it.
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
				// Thought marks the model's reasoning summary, which thinking
				// models stream before the reply. It is not the answer.
				Thought bool `json:"thought"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

func (r geminiResponse) text() string {
	var b strings.Builder
	for _, c := range r.Candidates {
		for _, p := range c.Content.Parts {
			if !p.Thought {
				b.WriteString(p.Text)
			}
		}
	}
	return b.String()
}

func (Gemini) reassemble(events []string) (answer string, promptTokens, responseTokens int) {
	var b strings.Builder
	for _, data := range events {
		if data == "" {
			continue
		}
		var chunk geminiResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		b.WriteString(chunk.text())
		// Usage is repeated on every chunk and grows; the last one is the total.
		if chunk.UsageMetadata.PromptTokenCount > 0 {
			promptTokens = chunk.UsageMetadata.PromptTokenCount
		}
		if chunk.UsageMetadata.CandidatesTokenCount > 0 {
			responseTokens = chunk.UsageMetadata.CandidatesTokenCount
		}
	}
	return b.String(), promptTokens, responseTokens
}

func (Gemini) parseWholeResponse(body []byte, res *Result) {
	var resp geminiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return
	}
	res.Answer = resp.text()
	res.PromptTokens = resp.UsageMetadata.PromptTokenCount
	res.ResponseTokens = resp.UsageMetadata.CandidatesTokenCount
}

// modelFromGeminiPath pulls "gemini-2.5-pro" out of
// "/v1beta/models/gemini-2.5-pro:streamGenerateContent".
func modelFromGeminiPath(path string) string {
	_, after, found := strings.Cut(path, "/models/")
	if !found {
		return ""
	}
	if i := strings.IndexAny(after, ":/"); i >= 0 {
		after = after[:i]
	}
	return after
}
