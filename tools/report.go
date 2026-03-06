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
		Description: "Save the report as a PDF. If content is empty, reads from the previously saved .md file with the same filename.",
	}, func(ctx tool.Context, args ReportArgs) (Result, error) {
		path, err := resolveReportPath(args.Filename, ".pdf")
		if err != nil {
			return Result{}, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Result{}, fmt.Errorf("creating reports directory: %w", err)
		}
		content := args.Content
		if content == "" && args.Filename != "" {
			mdPath, _ := resolveReportPath(args.Filename, ".md")
			if data, err := os.ReadFile(mdPath); err == nil {
				content = string(data)
			}
		}
		if err := renderMarkdownToPDF(args.Title, content, path); err != nil {
			return Result{}, fmt.Errorf("generating PDF: %w", err)
		}
		abs, _ := filepath.Abs(path)
		return Result{Summary: fmt.Sprintf("PDF report saved to %s", abs)}, nil
	})
	return t
}

type ReadReportArgs struct {
	Filename string `json:"filename" description:"Filename to read from the reports/ directory, with or without .md extension (e.g. 'repo-findings' or 'repo-findings.md')"`
}

func ListReports() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_reports",
		Description: "List all markdown report files available in the reports/ directory.",
	}, func(ctx tool.Context, args struct{}) (Result, error) {
		entries, err := os.ReadDir("reports")
		if os.IsNotExist(err) {
			return Result{Summary: "No reports directory found", Items: []Item{}, Issues: []string{}}, nil
		}
		if err != nil {
			return Result{}, fmt.Errorf("reading reports directory: %w", err)
		}
		items := []Item{}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				info, _ := e.Info()
				size := int64(0)
				if info != nil {
					size = info.Size()
				}
				items = append(items, Item{
					Name:    e.Name(),
					Status:  "available",
					Details: fmt.Sprintf("%d bytes", size),
				})
			}
		}
		return Result{
			Summary: fmt.Sprintf("%d report(s) available in reports/", len(items)),
			Items:   items,
			Issues:  []string{},
		}, nil
	})
	return t
}

func ReadReport() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "read_report",
		Description: "Read the full content of a markdown report file from the reports/ directory. Use list_reports first to see available files.",
	}, func(ctx tool.Context, args ReadReportArgs) (Result, error) {
		name := args.Filename
		if !strings.HasSuffix(name, ".md") {
			name += ".md"
		}
		name = filepath.Base(name) // prevent path traversal
		path := filepath.Join("reports", name)

		data, err := os.ReadFile(path)
		if err != nil {
			return Result{}, fmt.Errorf("reading report %s: %w", path, err)
		}
		return Result{
			Summary: fmt.Sprintf("Read %s (%d bytes)", path, len(data)),
			Items: []Item{{
				Name:    name,
				Status:  "ok",
				Details: string(data),
			}},
			Issues: []string{},
		}, nil
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
// Supported: # H1, ## H2, ### H3, - /* bullets, **bold** inline, tables, blank lines, ---.
func renderMarkdownToPDF(title, content, path string) error {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(22, 28, 22)
	pdf.SetAutoPageBreak(true, 18)

	today := time.Now().Format("January 2, 2006")

	// Footer: page number + date on every page
	pdf.SetFooterFunc(func() {
		pdf.SetY(-13)
		pdf.SetFont("Arial", "I", 8)
		pdf.SetTextColor(150, 150, 150)
		pdf.CellFormat(0, 8, fmt.Sprintf("Security & Deployment Audit  |  %s  |  Page %d", today, pdf.PageNo()), "", 0, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})

	pdf.AddPage()

	pageW, _ := pdf.GetPageSize()
	marginL, _, marginR, _ := pdf.GetMargins()
	contentW := pageW - marginL - marginR

	// ── Cover banner ─────────────────────────────────────────────────────────
	if title != "" {
		// Dark indigo banner
		pdf.SetFillColor(30, 27, 75)
		pdf.Rect(0, 0, pageW, 44, "F")
		pdf.SetFont("Arial", "B", 20)
		pdf.SetTextColor(255, 255, 255)
		pdf.SetXY(marginL, 10)
		pdf.CellFormat(contentW, 10, sanitizePDF(title), "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 9)
		pdf.SetTextColor(180, 180, 220)
		pdf.SetX(marginL)
		pdf.CellFormat(contentW, 6, "Security & Deployment Audit Report  |  "+today, "", 1, "L", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
		pdf.Ln(10)
	}

	// severity keyword → badge colours
	severityBadge := func(line string) (ok bool, fr, fg, fb, tr, tg, tb int, kw string) {
		up := strings.ToUpper(line)
		type entry struct {
			match        string
			fr, fg, fb   int
			tr, tg, tb   int
			label        string
		}
		entries := []entry{
			{"CRITICAL", 220, 38, 38, 255, 255, 255, "CRITICAL"},
			{"BLOCK", 185, 28, 28, 255, 255, 255, "BLOCK"},
			{" HIGH", 234, 88, 12, 255, 255, 255, "HIGH"},
			{"MEDIUM", 202, 138, 4, 255, 255, 255, "MEDIUM"},
			{"WARN", 180, 120, 0, 255, 255, 255, "WARN"},
			{"DEPLOYABLE", 21, 128, 61, 255, 255, 255, "PASS"},
			{"PASS", 21, 128, 61, 255, 255, 255, "PASS"},
			{" LOW", 71, 85, 105, 255, 255, 255, "LOW"},
		}
		for _, e := range entries {
			if strings.Contains(up, e.match) || strings.HasPrefix(up, strings.TrimSpace(e.match)) {
				return true, e.fr, e.fg, e.fb, e.tr, e.tg, e.tb, e.label
			}
		}
		return false, 0, 0, 0, 0, 0, 0, ""
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		switch {
		// ── H1 ──────────────────────────────────────────────────────────────
		case strings.HasPrefix(line, "# "):
			heading := sanitizePDF(strings.TrimPrefix(line, "# "))
			pdf.SetFillColor(30, 27, 75)
			pdf.SetTextColor(255, 255, 255)
			pdf.SetFont("Arial", "B", 14)
			pdf.SetX(marginL)
			pdf.MultiCell(contentW, 8, "  "+heading, "", "L", true)
			pdf.SetTextColor(0, 0, 0)
			pdf.Ln(3)

		// ── H2 ──────────────────────────────────────────────────────────────
		case strings.HasPrefix(line, "## "):
			heading := sanitizePDF(strings.TrimPrefix(line, "## "))
			pdf.SetFillColor(238, 237, 255)
			pdf.SetTextColor(30, 27, 75)
			pdf.SetFont("Arial", "B", 12)
			pdf.SetX(marginL)
			pdf.MultiCell(contentW, 7, "  "+heading, "", "L", true)
			pdf.SetTextColor(0, 0, 0)
			pdf.Ln(2)

		// ── H3 ──────────────────────────────────────────────────────────────
		case strings.HasPrefix(line, "### "):
			heading := sanitizePDF(strings.TrimPrefix(line, "### "))
			if ok, fr, fg, fb, tr, tg, tb, kw := severityBadge(heading); ok {
				pdf.SetFont("Arial", "B", 9)
				pdf.SetFillColor(fr, fg, fb)
				pdf.SetTextColor(tr, tg, tb)
				pdf.SetX(marginL)
				badgeW := 28.0
				pdf.CellFormat(badgeW, 6, " "+kw+" ", "0", 0, "C", true, 0, "")
				pdf.SetFillColor(255, 255, 255)
				pdf.SetTextColor(30, 27, 75)
				pdf.SetFont("Arial", "B", 11)
				pdf.MultiCell(contentW-badgeW, 6, "  "+heading, "", "L", false)
			} else {
				pdf.SetFont("Arial", "B", 11)
				pdf.SetTextColor(55, 48, 163)
				pdf.SetX(marginL)
				pdf.MultiCell(contentW, 6, heading, "", "L", false)
			}
			pdf.SetTextColor(0, 0, 0)
			pdf.Ln(1)

		// ── Bullet ──────────────────────────────────────────────────────────
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			body := strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* ")
			pdf.SetFont("Arial", "", 10)
			pdf.SetTextColor(30, 30, 30)
			pdf.SetX(marginL + 4)
			pdf.MultiCell(contentW-4, 5.5, "-  "+sanitizePDF(stripInlineMarkdown(body)), "", "L", false)
			pdf.SetTextColor(0, 0, 0)

		// ── Blank line ───────────────────────────────────────────────────────
		case strings.TrimSpace(line) == "":
			pdf.Ln(3)

		// ── Horizontal rule ──────────────────────────────────────────────────
		case strings.HasPrefix(line, "---") || strings.HasPrefix(line, "==="):
			pdf.SetDrawColor(200, 200, 220)
			pdf.Line(marginL, pdf.GetY()+1, pageW-marginR, pdf.GetY()+1)
			pdf.SetDrawColor(0, 0, 0)
			pdf.Ln(4)

		// ── Table row (| col | col |) ────────────────────────────────────────
		case strings.HasPrefix(strings.TrimSpace(line), "|") && strings.Contains(line, "|"):
			if strings.Contains(line, "---") {
				continue // separator row
			}
			cols := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
			colW := contentW / float64(len(cols))
			// Heuristic: header row if the previous cell was at marginL (first row after separator)
			isHeader := false
			for _, c := range cols {
				if strings.TrimSpace(c) != "" {
					t := strings.ToUpper(strings.TrimSpace(c))
					if t == "SEVERITY" || t == "CHECK" || t == "FINDING" || t == "CATEGORY" || t == "COUNT" || t == "RESULT" {
						isHeader = true
					}
					break
				}
			}
			if isHeader {
				pdf.SetFillColor(30, 27, 75)
				pdf.SetTextColor(255, 255, 255)
				pdf.SetFont("Arial", "B", 9)
			} else {
				pdf.SetFillColor(248, 248, 255)
				pdf.SetTextColor(20, 20, 60)
				pdf.SetFont("Arial", "", 9)
			}
			pdf.SetX(marginL)
			for _, col := range cols {
				pdf.CellFormat(colW, 6, " "+sanitizePDF(strings.TrimSpace(col)), "1", 0, "L", true, 0, "")
			}
			pdf.Ln(-1)
			pdf.SetTextColor(0, 0, 0)

		// ── Default paragraph ────────────────────────────────────────────────
		default:
			pdf.SetFont("Arial", "", 10)
			pdf.SetTextColor(40, 40, 40)
			pdf.SetX(marginL)
			pdf.MultiCell(contentW, 5.5, sanitizePDF(stripInlineMarkdown(line)), "", "L", false)
			pdf.SetTextColor(0, 0, 0)
		}
	}

	return pdf.OutputFileAndClose(path)
}

// ExportTestPDF is a test/dev helper that calls renderMarkdownToPDF directly.
func ExportTestPDF(title, content, path string) error {
	return renderMarkdownToPDF(title, content, path)
}

// ConvertMarkdownFileToPDF reads an existing markdown file and renders it as PDF.
func ConvertMarkdownFileToPDF() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "convert_markdown_file_to_pdf",
		Description: "Read an existing markdown (.md) file from disk and convert it to a PDF. Returns the path of the generated PDF.",
	}, func(ctx tool.Context, args struct {
		MarkdownPath string `json:"markdown_path" description:"Path to the existing markdown file (e.g. reports/cluster-report-2026-03-05.md)"`
		Title        string `json:"title" description:"Optional title to display at the top of the PDF. If empty, the filename is used."`
	}) (Result, error) {
		content, err := os.ReadFile(args.MarkdownPath)
		if err != nil {
			return Result{}, fmt.Errorf("reading markdown file %s: %w", args.MarkdownPath, err)
		}

		title := args.Title
		if title == "" {
			title = filepath.Base(args.MarkdownPath)
		}

		// Derive PDF path: same directory and base name, .pdf extension
		base := strings.TrimSuffix(args.MarkdownPath, filepath.Ext(args.MarkdownPath))
		pdfPath := base + ".pdf"

		if err := os.MkdirAll(filepath.Dir(pdfPath), 0o755); err != nil {
			return Result{}, fmt.Errorf("creating output directory: %w", err)
		}
		if err := renderMarkdownToPDF(title, string(content), pdfPath); err != nil {
			return Result{}, fmt.Errorf("rendering PDF: %w", err)
		}

		abs, _ := filepath.Abs(pdfPath)
		return Result{Summary: fmt.Sprintf("PDF generated at %s", abs)}, nil
	})
	return t
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
