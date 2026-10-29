package parser

import "ragflow/internal/utility"

// ParserCapability describes registration, not runtime readiness or extraction quality.
// Model-assisted ingestion and knowledge representation eligibility are checked by
// their owning services rather than inferred from a file suffix.
type ParserCapability struct {
	FileType   utility.FileType `json:"file_type"`
	BaseResult string           `json:"base_result"`
}

type parserRegistration struct {
	capability ParserCapability
	create     func() (ParseResultProducer, error)
}

var parserRegistrations = []parserRegistration{
	{ParserCapability{utility.FileTypePPTX, "structured"}, func() (ParseResultProducer, error) { return NewPPTXParser(), nil }},
	{ParserCapability{utility.FileTypePPT, "structured"}, func() (ParseResultProducer, error) { return NewPPTParser(), nil }},
	{ParserCapability{utility.FileTypeXLSX, "structured"}, func() (ParseResultProducer, error) { return NewXLSXParser("") }},
	{ParserCapability{utility.FileTypeXLS, "structured"}, func() (ParseResultProducer, error) { return NewXLSParser("") }},
	{ParserCapability{utility.FileTypeCSV, "structured"}, func() (ParseResultProducer, error) { return NewCSVParser(), nil }},
	{ParserCapability{utility.FileTypeDOCX, "structured"}, func() (ParseResultProducer, error) { return NewDOCXParser(), nil }},
	{ParserCapability{utility.FileTypeDOC, "structured"}, func() (ParseResultProducer, error) { return NewDOCParser(), nil }},
	{ParserCapability{utility.FileTypePDF, "structured"}, func() (ParseResultProducer, error) { return NewPDFParser(), nil }},
	{ParserCapability{utility.FileTypeHTML, "structured"}, func() (ParseResultProducer, error) { return NewHTMLParser(), nil }},
	{ParserCapability{utility.FileTypeMarkdown, "structured"}, func() (ParseResultProducer, error) { return NewMarkdownParser(GoMarkdown) }},
	{ParserCapability{utility.FileTypeTXT, "structured"}, func() (ParseResultProducer, error) { return NewTextParser(), nil }},
	{ParserCapability{utility.FileTypeEPUB, "structured"}, func() (ParseResultProducer, error) { return NewEPUBParser(), nil }},
	{ParserCapability{utility.FileTypeJSON, "structured"}, func() (ParseResultProducer, error) { return NewJSONParser(), nil }},
	{ParserCapability{utility.FileTypeEMAIL, "structured"}, func() (ParseResultProducer, error) { return NewEmailParser(), nil }},
	{ParserCapability{utility.FileTypeVISUAL, "image_payload"}, func() (ParseResultProducer, error) { return NewPictureParser(), nil }},
	{ParserCapability{utility.FileTypeAURAL, "configuration_only"}, func() (ParseResultProducer, error) { return NewAudioParser(), nil }},
}

// RegisteredParserCapabilities returns an independent ordered copy of the same
// registry used by GetParser. A structured result may be empty, unsupported or
// dependent on native libraries; callers must still inspect ParseResult.
func RegisteredParserCapabilities() []ParserCapability {
	capabilities := make([]ParserCapability, len(parserRegistrations))
	for i, registration := range parserRegistrations {
		capabilities[i] = registration.capability
	}
	return capabilities
}
