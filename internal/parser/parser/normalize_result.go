package parser

// NormalizedParseResult separates an empty or failed parse from a structured
// result. Fixed publication must reject failed and no_output.
type NormalizedParseResult struct {
	Result     ParseResult
	Status     string
	Diagnostic error
}

// NormalizeParseResult classifies an already-produced result. Parsers
// normalize their own Markdown and text into JSON items, so this never
// transcribes audio, performs OCR or fetches a remote Markdown image.
func NormalizeParseResult(input ParseResult) NormalizedParseResult {
	output := NormalizedParseResult{Result: input}
	if input.Err != nil {
		output.Status, output.Diagnostic = "failed", input.Err
		return output
	}
	if len(input.JSON) > 0 {
		output.Status = "json"
		return output
	}
	output.Status, output.Result.JSON = "no_output", []DocItem{}
	return output
}
