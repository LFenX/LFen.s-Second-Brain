//go:build !cgo

package parser

const documentStructurePDFReady = false

func (b *documentStructureBuilder) buildPDF([]byte) error {
	return documentFailure("source_structure_unavailable")
}
