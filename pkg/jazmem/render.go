package jazmem

import (
	"fmt"
	"strings"
)

func RenderSearchText(response SearchResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Results: %d pages, %d matched chunks\n\n", response.Stats.Pages, response.Stats.Chunks)
	if len(response.Results) == 0 {
		b.WriteString("No matching memory chunks were found.\n")
		return b.String()
	}
	for i, result := range response.Results {
		fmt.Fprintf(&b, "[%d] %s (%s)%s\n", i+1, result.Title, result.Slug, resultProvenance(result))
		for _, match := range result.Matches {
			if strings.TrimSpace(match.Snippet) == "" {
				continue
			}
			fmt.Fprintf(&b, "  chunk %d: %s\n", match.Chunk, strings.TrimSpace(match.Snippet))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func resultProvenance(result Result) string {
	parts := make([]string, 0, 2)
	if result.Via != "" {
		parts = append(parts, "via "+result.Via)
	}
	if !result.ModifiedAt.IsZero() {
		parts = append(parts, result.ModifiedAt.Format("2006-01-02"))
	}
	if len(parts) == 0 {
		return ""
	}
	return " [" + strings.Join(parts, ", ") + "]"
}

func RenderAgenticText(response AgenticResponse) string {
	var b strings.Builder
	b.WriteString(response.Answer)
	if !strings.HasSuffix(response.Answer, "\n") {
		b.WriteString("\n")
	}
	if len(response.Citations) > 0 {
		b.WriteString("\nSources:\n")
		for _, citation := range response.Citations {
			fmt.Fprintf(&b, "[%d] %s", citation.ID, citation.Slug)
			if citation.Title != "" {
				fmt.Fprintf(&b, " — %s", citation.Title)
			}
			b.WriteString("\n")
		}
	}
	if len(response.Gaps) > 0 {
		b.WriteString("\nGaps:\n")
		for _, gap := range response.Gaps {
			fmt.Fprintf(&b, "- %s\n", gap)
		}
	}
	return b.String()
}

// RenderTasksText groups tasks under their status (already ordered by ListTasks)
// so a person can read the working set at a glance.
func RenderTasksText(tasks []Task) string {
	if len(tasks) == 0 {
		return "No tasks.\n"
	}
	var b strings.Builder
	status := ""
	for _, task := range tasks {
		if task.Status != status {
			if status != "" {
				b.WriteString("\n")
			}
			status = task.Status
			fmt.Fprintf(&b, "%s\n", strings.ToUpper(status))
		}
		fmt.Fprintf(&b, "  %s  (%s)", task.Title, task.Slug)
		if task.Project != "" {
			fmt.Fprintf(&b, " → %s", task.Project)
		}
		b.WriteString("\n")
	}
	return b.String()
}
