package pdf

import (
	"bytes"
	"io"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/russross/blackfriday/v2"
)

// MarkdownToPDF converts markdown text to PDF and writes to the provided writer
func MarkdownToPDF(markdown string, w io.Writer) error {
	// Parse markdown to HTML
	html := blackfriday.Run([]byte(markdown))

	// Create a new PDF document
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "", 12)

	// Process the HTML content
	lines := strings.Split(string(html), "\n")
	for _, line := range lines {
		// Handle headings (# Heading)
		if strings.HasPrefix(line, "<h1>") && strings.HasSuffix(line, "</h1>") {
			heading := stripTags(line)
			pdf.SetFont("Arial", "B", 16)
			pdf.Cell(0, 10, heading)
			pdf.Ln(15)
			pdf.SetFont("Arial", "", 12)
		} else if strings.HasPrefix(line, "<h2>") && strings.HasSuffix(line, "</h2>") {
			heading := stripTags(line)
			pdf.SetFont("Arial", "B", 14)
			pdf.Cell(0, 10, heading)
			pdf.Ln(10)
			pdf.SetFont("Arial", "", 12)
		} else if len(line) > 0 && !strings.HasPrefix(line, "<") {
			// Regular text
			pdf.MultiCell(0, 5, line, "", "", false)
			pdf.Ln(5)
		}
	}

	// Write the PDF to the output writer
	return pdf.Output(w)
}

// stripTags removes HTML tags from a string
func stripTags(html string) string {
	var buf bytes.Buffer
	var inTag bool

	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			buf.WriteRune(r)
		}
	}

	return strings.TrimSpace(buf.String())
}
