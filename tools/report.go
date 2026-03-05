package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type ReportArgs struct {
	Title    string `json:"title" description:"Title of the report"`
	Content  string `json:"content" description:"Full markdown content of the report"`
	Filename string `json:"filename" description:"Base filename without extension (e.g. cluster-report-2024-01-15). If empty, a timestamped name is used."`
}

func SaveReportMarkdown() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "save_report_markdown",
		Description: "Save the client report as a markdown (.md) file. Returns the file path written.",
	}, func(ctx tool.Context, args ReportArgs) (Result, error) {
		path, err := resolveReportPath(args.Filename, ".md")
		if err != nil {
			return Result{}, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Result{}, fmt.Errorf("creating reports directory: %w", err)
		}
		if err := os.WriteFile(path, []byte(args.Content), 0o644); err != nil {
			return Result{}, fmt.Errorf("writing markdown report: %w", err)
		}
		abs, _ := filepath.Abs(path)
		return Result{Summary: fmt.Sprintf("Markdown report saved to %s", abs)}, nil
	})
	return t
}

func SaveReportPDF() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "save_report_pdf",
		Description: "Save the client report as a PDF file. Returns the file path written.",
	}, func(ctx tool.Context, args ReportArgs) (Result, error) {
		path, err := resolveReportPath(args.Filename, ".pdf")
		if err != nil {
			return Result{}, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Result{}, fmt.Errorf("creating reports directory: %w", err)
		}
		if err := renderMarkdownToPDF(args.Title, args.Content, path); err != nil {
			return Result{}, fmt.Errorf("generating PDF: %w", err)
		}
		abs, _ := filepath.Abs(path)
		return Result{Summary: fmt.Sprintf("PDF report saved to %s", abs)}, nil
	})
	return t
}

// resolveReportPath returns reports/<filename><ext>, generating a timestamped name if empty.
func resolveReportPath(filename, ext string) (string, error) {
	if filename == "" {
		filename = fmt.Sprintf("cluster-report-%s", time.Now().Format("2006-01-02T15-04-05"))
	}
	// Sanitise: strip any path separators the LLM might inject
	filename = filepath.Base(filename)
	return filepath.Join("reports", filename+ext), nil
}

// renderMarkdownToPDF converts a subset of markdown to a PDF using go-pdf/fpdf.
// Supported syntax: # H1, ## H2, ### H3, - / * bullets, **bold** inline, blank lines.
func renderMarkdownToPDF(title, content, path string) error {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)
	pdf.AddPage()

	pageW, _ := pdf.GetPageSize()
	marginL, _, marginR, _ := pdf.GetMargins()
	contentW := pageW - marginL - marginR

	// Cover header
	if title != "" {
		pdf.SetFont("Arial", "B", 22)
		pdf.MultiCell(contentW, 10, title, "", "C", false)
		pdf.Ln(6)
		pdf.SetDrawColor(100, 100, 200)
		pdf.Line(marginL, pdf.GetY(), pageW-marginR, pdf.GetY())
		pdf.Ln(6)
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "# "):
			pdf.SetFont("Arial", "B", 18)
			pdf.SetTextColor(30, 30, 100)
			pdf.MultiCell(contentW, 9, sanitizePDF(strings.TrimPrefix(line, "# ")), "", "L", false)
			pdf.SetTextColor(0, 0, 0)
			pdf.Ln(3)

		case strings.HasPrefix(line, "## "):
			pdf.SetFont("Arial", "B", 14)
			pdf.SetTextColor(40, 40, 130)
			pdf.MultiCell(contentW, 8, sanitizePDF(strings.TrimPrefix(line, "## ")), "", "L", false)
			pdf.SetTextColor(0, 0, 0)
			pdf.Ln(2)

		case strings.HasPrefix(line, "### "):
			pdf.SetFont("Arial", "B", 12)
			pdf.SetTextColor(60, 60, 150)
			pdf.MultiCell(contentW, 7, sanitizePDF(strings.TrimPrefix(line, "### ")), "", "L", false)
			pdf.SetTextColor(0, 0, 0)
			pdf.Ln(1)

		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			text := strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* ")
			pdf.SetFont("Arial", "", 10)
			pdf.SetX(marginL + 5)
			pdf.MultiCell(contentW-5, 5.5, "-  "+sanitizePDF(stripInlineMarkdown(text)), "", "L", false)

		case strings.TrimSpace(line) == "":
			pdf.Ln(4)

		case strings.HasPrefix(line, "---") || strings.HasPrefix(line, "==="):
			pdf.SetDrawColor(180, 180, 180)
			pdf.Line(marginL, pdf.GetY(), pageW-marginR, pdf.GetY())
			pdf.Ln(3)

		default:
			pdf.SetFont("Arial", "", 10)
			pdf.MultiCell(contentW, 5.5, sanitizePDF(stripInlineMarkdown(line)), "", "L", false)
		}
	}

	return pdf.OutputFileAndClose(path)
}

// sanitizePDF converts non-Latin1 characters to ASCII equivalents so that
// go-pdf/fpdf's built-in Arial font (ISO-8859-1) renders them correctly.
func sanitizePDF(s string) string {
	replacer := strings.NewReplacer(
		"\u2022", "-",  // bullet •
		"\u2013", "-",  // en dash –
		"\u2014", "--", // em dash —
		"\u2018", "'",  // left single quote '
		"\u2019", "'",  // right single quote '
		"\u201c", "\"", // left double quote "
		"\u201d", "\"", // right double quote "
		"\u2026", "...", // ellipsis …
		"\u00a0", " ",  // non-breaking space
		"\u2192", "->", // arrow →
		"\u2190", "<-", // arrow ←
		"\u2605", "*",  // star ★
		"\u2713", "v",  // check mark ✓
		"\u2717", "x",  // cross ✗
	)
	s = replacer.Replace(s)
	// Strip any remaining non-Latin1 characters
	var out strings.Builder
	for _, r := range s {
		if r <= 0xFF {
			out.WriteRune(r)
		} else {
			out.WriteByte('?')
		}
	}
	return out.String()
}

// stripInlineMarkdown removes common inline markdown tokens (**bold**, *italic*, `code`).
func stripInlineMarkdown(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "`", "")
	// Remove single * only when not part of a bullet (already handled above)
	var out strings.Builder
	skip := false
	for i := 0; i < len(s); i++ {
		if s[i] == '*' && !skip {
			skip = true
			continue
		}
		if s[i] == '*' && skip {
			skip = false
			continue
		}
		out.WriteByte(s[i])
	}
	return strings.TrimSpace(out.String())
}
