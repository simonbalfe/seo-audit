// Package reportpdf renders client-facing local visibility reports.
package reportpdf

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/simonbalfe/seo-audit/internal/report"
)

const (
	ink   = "15,23,42"
	muted = "100,116,139"
	blue  = "37,99,235"
)

// Render creates a client-facing PDF from DataForSEO visibility data.
func Render(data report.SiteReport) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-11)
		setColor(pdf, muted)
		pdf.SetFont("Arial", "", 8)
		pdf.CellFormat(0, 5, "Rankings are location- and time-specific and use public data.", "", 0, "C", false, 0, "")
	})
	pdf.AddPage()
	setColor(pdf, blue)
	pdf.SetFont("Arial", "B", 10)
	pdf.Cell(0, 6, "LOCAL SEARCH VISIBILITY")
	pdf.Ln(9)
	setColor(pdf, ink)
	pdf.SetFont("Arial", "B", 24)
	name := "SEO visibility report"
	if data.GBP != nil && strings.TrimSpace(data.GBP.Name) != "" {
		name = data.GBP.Name
	}
	pdf.MultiCell(0, 10, name, "", "L", false)
	pdf.SetFont("Arial", "", 10)
	setColor(pdf, muted)
	details := []string{data.Market.Location, data.StartURL, time.Now().Format("2 January 2006")}
	pdf.MultiCell(0, 6, strings.Join(nonempty(details), "  |  "), "", "L", false)
	if data.GBP != nil {
		profile := nonempty([]string{data.GBP.Category, data.GBP.Address})
		if len(profile) > 0 {
			pdf.MultiCell(0, 6, strings.Join(profile, "  |  "), "", "L", false)
		}
	}
	pdf.Ln(5)
	metrics(pdf, data)
	section(pdf, "Current search visibility")
	visibilityTable(pdf, append(append([]report.Opportunity{}, data.Market.CurrentVisibility...), data.Market.Opportunities...))
	maps := append(append([]report.MapsVisibility{}, data.Market.CurrentMaps...), data.Market.OpportunityMaps...)
	if len(maps) > 0 {
		section(pdf, "Google Maps visibility")
		for _, item := range maps {
			mapsBlock(pdf, item)
		}
	}
	if len(data.Market.Opportunities) > 0 {
		section(pdf, "Recommended opportunities")
		opportunities(pdf, data.Market.Opportunities)
	}
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return nil, fmt.Errorf("render visibility PDF: %w", err)
	}
	return output.Bytes(), nil
}

func metrics(pdf *fpdf.Fpdf, data report.SiteReport) {
	values := [][2]string{
		{fmt.Sprint(len(data.Market.ExistingRankings)), "Existing rankings"},
		{fmt.Sprint(len(data.Market.CurrentVisibility)), "Current terms checked"},
		{fmt.Sprint(len(data.Market.Opportunities)), "Growth opportunities"},
		{fmt.Sprint(len(data.Market.CurrentMaps) + len(data.Market.OpportunityMaps)), "Maps checks"},
	}
	x, y := pdf.GetXY()
	width := 44.5
	for index, value := range values {
		pdf.SetXY(x+float64(index)*width, y)
		pdf.SetFillColor(241, 245, 249)
		pdf.Rect(pdf.GetX(), pdf.GetY(), width-2, 22, "F")
		pdf.SetXY(pdf.GetX()+4, pdf.GetY()+3)
		setColor(pdf, ink)
		pdf.SetFont("Arial", "B", 16)
		pdf.Cell(width-8, 7, value[0])
		pdf.SetXY(pdf.GetX()-(width-8), pdf.GetY()+8)
		setColor(pdf, muted)
		pdf.SetFont("Arial", "", 8)
		pdf.Cell(width-8, 5, value[1])
	}
	pdf.SetXY(x, y+27)
}

func section(pdf *fpdf.Fpdf, title string) {
	pdf.Ln(4)
	setColor(pdf, ink)
	pdf.SetFont("Arial", "B", 15)
	pdf.Cell(0, 8, title)
	pdf.Ln(10)
}

func visibilityTable(pdf *fpdf.Fpdf, rows []report.Opportunity) {
	unique := make(map[string]report.Opportunity)
	for _, row := range rows {
		if _, exists := unique[row.Keyword]; !exists && strings.TrimSpace(row.Keyword) != "" {
			unique[row.Keyword] = row
		}
	}
	rows = rows[:0]
	for _, row := range unique {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].SearchVolume > rows[j].SearchVolume })
	if len(rows) == 0 {
		setColor(pdf, muted)
		pdf.SetFont("Arial", "", 10)
		pdf.Cell(0, 6, "No keyword visibility results were returned for this location.")
		pdf.Ln(8)
		return
	}
	tableHeader(pdf, []string{"Keyword", "Organic", "Maps", "Monthly searches"}, []float64{92, 27, 25, 36})
	for index, row := range rows {
		if index == 12 {
			break
		}
		tableRow(pdf, []string{row.Keyword, rank(row.Position), rank(row.MapsPosition), fmt.Sprint(row.SearchVolume)}, []float64{92, 27, 25, 36})
	}
}

func mapsBlock(pdf *fpdf.Fpdf, item report.MapsVisibility) {
	if pdf.GetY() > 230 {
		pdf.AddPage()
	}
	setColor(pdf, ink)
	pdf.SetFont("Arial", "B", 11)
	pdf.Cell(0, 7, item.Keyword)
	pdf.Ln(7)
	setColor(pdf, muted)
	pdf.SetFont("Arial", "", 9)
	pdf.Cell(0, 5, fmt.Sprintf("Centre rank %s  |  Top 3 coverage %.0f%%  |  Median rank %s", rank(item.TargetPosition), item.TopThreeCoverage, rank(item.MedianPosition)))
	pdf.Ln(7)
	if len(item.GridPoints) == 9 {
		x, y := pdf.GetXY()
		for index, point := range item.GridPoints {
			column, row := index%3, index/3
			pdf.SetXY(x+float64(column)*18, y+float64(row)*12)
			pdf.SetFillColor(241, 245, 249)
			pdf.SetDrawColor(226, 232, 240)
			pdf.SetFont("Arial", "B", 10)
			setColor(pdf, ink)
			value := rank(point.Position)
			if point.Status == "error" {
				value = "X"
			}
			pdf.CellFormat(16, 10, value, "1", 0, "C", true, 0, "")
		}
		pdf.SetXY(x, y+39)
	}
	pdf.Ln(3)
}

func opportunities(pdf *fpdf.Fpdf, rows []report.Opportunity) {
	for index, item := range rows {
		if index == 8 {
			break
		}
		if pdf.GetY() > 255 {
			pdf.AddPage()
		}
		setColor(pdf, ink)
		pdf.SetFont("Arial", "B", 10)
		pdf.Cell(0, 6, item.Keyword)
		pdf.Ln(6)
		setColor(pdf, muted)
		pdf.SetFont("Arial", "", 9)
		line := fmt.Sprintf("%d monthly searches  |  Organic %s  |  Maps %s", item.SearchVolume, rank(item.Position), rank(item.MapsPosition))
		if len(item.Actions) > 0 {
			line += "  |  " + item.Actions[0]
		}
		pdf.MultiCell(0, 5, line, "", "L", false)
		pdf.Ln(2)
	}
}

func tableHeader(pdf *fpdf.Fpdf, values []string, widths []float64) {
	pdf.SetFillColor(226, 232, 240)
	setColor(pdf, ink)
	pdf.SetFont("Arial", "B", 8)
	for index, value := range values {
		pdf.CellFormat(widths[index], 8, value, "", 0, "L", true, 0, "")
	}
	pdf.Ln(8)
}

func tableRow(pdf *fpdf.Fpdf, values []string, widths []float64) {
	setColor(pdf, ink)
	pdf.SetFont("Arial", "", 8)
	for index, value := range values {
		pdf.CellFormat(widths[index], 7, truncate(value, 54), "B", 0, "L", false, 0, "")
	}
	pdf.Ln(7)
}

func rank(value int) string {
	if value <= 0 {
		return "Not found"
	}
	return fmt.Sprintf("#%d", value)
}

func nonempty(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func truncate(value string, limit int) string {
	characters := []rune(value)
	if len(characters) <= limit {
		return value
	}
	return string(characters[:limit-1]) + "..."
}

func setColor(pdf *fpdf.Fpdf, value string) {
	var red, green, blue int
	_, _ = fmt.Sscanf(value, "%d,%d,%d", &red, &green, &blue)
	pdf.SetTextColor(red, green, blue)
}
