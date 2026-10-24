package file

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"gorm.io/gorm"
)

// This exercises the existing byte/snapshot primitive, not C2 binary admission,
// document parsing, Office rendering, authorization or HTTP download support.
func TestKnowledgeSourcePreparationOriginals(t *testing.T) {
	for _, kind := range []string{"pdf", "pptx", "docx", "bin"} {
		t.Run(kind, func(t *testing.T) {
			db := setupFolderTestDB(t)
			if err := db.AutoMigrate(&entity.FileCommit{}, &entity.FileCommitItem{}); err != nil {
				t.Fatal(err)
			}
			folder := entity.File{ID: "synthetic-originals", ParentID: "synthetic-originals", TenantID: "owner", CreatedBy: "owner", Name: "Synthetic originals", Type: "folder"}
			source := entity.File{ID: "synthetic-source", ParentID: folder.ID, TenantID: "owner", CreatedBy: "owner", Name: "sample." + kind, Type: kind}
			for _, row := range []entity.File{folder, source} {
				if err := db.Create(&row).Error; err != nil {
					t.Fatal(err)
				}
			}
			factory := storage.GetStorageFactory()
			previous := factory.GetStorage()
			memory := storage.NewMemoryStorage()
			factory.SetStorage(memory)
			t.Cleanup(func() { factory.SetStorage(previous) })
			svc := NewFileCommitService()
			firstBytes, secondBytes := preparationOriginal(t, kind, "first"), preparationOriginal(t, kind, "second")
			var first, second *entity.FileCommit
			for i, body := range [][]byte{firstBytes, secondBytes} {
				var commit *entity.FileCommit
				err := db.Transaction(func(tx *gorm.DB) error {
					var err error
					commit, _, err = svc.RecordSourceSnapshot(t.Context(), tx, &source, "owner", body)
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					first = commit
				} else {
					second = commit
				}
			}
			if second.ParentID == nil || *second.ParentID != first.ID {
				t.Fatal("native predecessor lost")
			}
			for i, commit := range []*entity.FileCommit{first, second} {
				actual, err := svc.GetCommitFileContent(t.Context(), folder.ID, commit.ID, source.ID)
				if err != nil || !bytes.Equal(actual, [][]byte{firstBytes, secondBytes}[i]) {
					t.Fatalf("original bytes changed: %v", err)
				}
			}
			// A failed object write must not advance the native DB head.
			factory.SetStorage(preparationOriginalWriteFailure{Storage: memory})
			if err := db.Transaction(func(tx *gorm.DB) error {
				_, _, err := svc.RecordSourceSnapshot(t.Context(), tx, &source, "owner", preparationOriginal(t, kind, "failed-write"))
				return err
			}); err == nil {
				t.Fatal("failed write reported success")
			}
			factory.SetStorage(memory)
			// Object writes precede the SQL transaction. A later rollback may leave
			// an unreferenced object; it must not publish a commit or delete history.
			rollback := errors.New("synthetic metadata rollback")
			if err := db.Transaction(func(tx *gorm.DB) error {
				if _, _, err := svc.RecordSourceSnapshot(t.Context(), tx, &source, "owner", preparationOriginal(t, kind, "orphan")); err != nil {
					return err
				}
				return rollback
			}); !errors.Is(err, rollback) {
				t.Fatal(err)
			}
			head, err := dao.NewFileCommitDAO().GetLatestByFolderID(t.Context(), db, folder.ID)
			if err != nil || head.ID != second.ID {
				t.Fatalf("failure advanced head: %+v %v", head, err)
			}
			var commits, items int64
			if err := db.Model(&entity.FileCommit{}).Count(&commits).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&entity.FileCommitItem{}).Count(&items).Error; err != nil {
				t.Fatal(err)
			}
			if commits != 2 || items != 2 {
				t.Fatalf("rollback published rows: %d %d", commits, items)
			}
			old, err := svc.GetCommitFileContent(t.Context(), folder.ID, first.ID, source.ID)
			if err != nil || !bytes.Equal(old, firstBytes) {
				t.Fatal("failure damaged old revision")
			}
			objects, err := memory.ListObjects(t.Context(), folder.ID)
			if err != nil || len(objects) != 3 {
				t.Fatalf("expected two referenced and one retained orphan object: %d %v", len(objects), err)
			}
		})
	}
}

type preparationOriginalWriteFailure struct{ storage.Storage }

func (s preparationOriginalWriteFailure) Put(context.Context, string, string, []byte, ...string) error {
	return errors.New("synthetic object write failure")
}

func preparationOriginal(t *testing.T, kind, label string) []byte {
	t.Helper()
	switch kind {
	case "pdf":
		var body bytes.Buffer
		body.WriteString("%PDF-1.4\n")
		stream := "BT /F1 12 Tf 40 100 Td (Synthetic " + label + ") Tj ET\n"
		objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream)}
		offsets := []int{0}
		for i, obj := range objects {
			offsets = append(offsets, body.Len())
			fmt.Fprintf(&body, "%d 0 obj\n%s\nendobj\n", i+1, obj)
		}
		xref := body.Len()
		fmt.Fprintf(&body, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
		for _, offset := range offsets[1:] {
			fmt.Fprintf(&body, "%010d 00000 n \n", offset)
		}
		fmt.Fprintf(&body, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
		return body.Bytes()
	case "docx", "pptx":
		mainPart, contentType, mainXML := "word/document.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml", `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Synthetic `+label+` 合成资料</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`
		if kind == "pptx" {
			mainPart, contentType, mainXML = "ppt/presentation.xml", "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml", `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId id="256" r:id="rId1"/></p:sldIdLst><p:sldSz cx="9144000" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/></p:presentation>`
		}
		parts := map[string]string{
			"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/` + mainPart + `" ContentType="` + contentType + `"/>` + map[bool]string{true: `<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`}[kind == "pptx"] + `</Types>`,
			"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="` + mainPart + `"/></Relationships>`,
			mainPart:              mainXML,
		}
		if kind == "pptx" {
			parts["ppt/_rels/presentation.xml.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/></Relationships>`
			parts["ppt/slides/slide1.xml"] = `<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/><p:sp><p:nvSpPr><p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>Synthetic ` + label + ` 合成资料</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`
		}
		var body bytes.Buffer
		writer := zip.NewWriter(&body)
		// Fixed order and default ZIP timestamps keep synthetic bytes repeatable.
		for _, name := range []string{"[Content_Types].xml", "_rels/.rels", mainPart, "ppt/_rels/presentation.xml.rels", "ppt/slides/slide1.xml"} {
			value, exists := parts[name]
			if !exists {
				continue
			}
			entry, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(entry, value); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		reader, err := zip.NewReader(bytes.NewReader(body.Bytes()), int64(body.Len()))
		if err != nil {
			t.Fatal(err)
		}
		for _, part := range reader.File {
			file, err := part.Open()
			if err != nil {
				t.Fatal(err)
			}
			decoder := xml.NewDecoder(file)
			for {
				_, err := decoder.Token()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
		}
		return body.Bytes()
	default:
		return append([]byte{0, 255, 0, 17}, []byte(strings.Repeat(label, 3))...)
	}
}
