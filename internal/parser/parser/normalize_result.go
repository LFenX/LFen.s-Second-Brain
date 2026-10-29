package parser

import (
	"context"
	"fmt"
)

// NormalizedParseResult separates the component's text fallback from successful
// normalization. Fixed publication must reject fallback_text, failed and no_output.
type NormalizedParseResult struct {
	Result      ParseResult
	InputFormat string
	Status      string
	Diagnostic  error
}

// NormalizeParseResult owns local normalization of an already-produced result.
// It never transcribes audio, performs OCR or fetches a remote Markdown image.
func NormalizeParseResult(ctx context.Context, filename string, input ParseResult) NormalizedParseResult {
	output := NormalizedParseResult{Result: input, InputFormat: input.OutputFormat}
	if input.Err != nil {
		output.Status, output.Diagnostic = "failed", input.Err
		return output
	}
	if input.OutputFormat == "" {
		output.Status, output.Diagnostic = "failed", fmt.Errorf("missing parser output format")
		return output
	}
	set := func(status string, items []DocItem) NormalizedParseResult {
		output.Status, output.Result.OutputFormat, output.Result.JSON = status, "json", items
		output.Result.Markdown, output.Result.Text = "", ""
		return output
	}
	if len(input.JSON) > 0 {
		return set("json", input.JSON)
	}
	if input.Markdown != "" {
		markdown, err := NewMarkdownParser(GoMarkdown)
		if err == nil {
			markdown.FetchRemoteImages = false
			parsed := markdown.ParseWithResult(ctx, filename, []byte(input.Markdown))
			err = parsed.Err
			if err == nil && len(parsed.JSON) > 0 {
				return set("markdown", parsed.JSON)
			}
			if err == nil {
				err = fmt.Errorf("parser returned no JSON items")
			}
		}
		output.Diagnostic = err
		return set("fallback_text", []DocItem{NewTextJSONItem(input.Markdown)})
	}
	if input.Text != "" {
		return set("text", []DocItem{NewTextJSONItem(input.Text)})
	}
	return set("no_output", []DocItem{})
}
