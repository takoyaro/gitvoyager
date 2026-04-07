package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/takoyaro/gitvoyager/internal/model"
	"github.com/takoyaro/gitvoyager/internal/store"
)

// ── Exclude Picker (inline status bar mode) ──

type excludePickerItem struct {
	label string // display label: `owner "hashicorp"`
	kind  string // "owner", "topic", "keyword"
	value string // the actual value to exclude
}

// buildPickerItems creates the picker options from the selected repo.
func buildPickerItems(r *model.Repo) []excludePickerItem {
	var items []excludePickerItem

	// Always offer owner
	items = append(items, excludePickerItem{
		label: fmt.Sprintf("owner \"%s\"", r.Owner),
		kind:  "owner",
		value: r.Owner,
	})

	// Topics (only if enriched, max 4)
	shown := 0
	for _, t := range r.Topics {
		if shown >= 4 {
			break
		}
		items = append(items, excludePickerItem{
			label: fmt.Sprintf("topic \"%s\"", t),
			kind:  "topic",
			value: t,
		})
		shown++
	}

	// Always offer keyword entry as last option
	items = append(items, excludePickerItem{
		label: "keyword…",
		kind:  "keyword",
		value: "",
	})

	return items
}

// renderPicker renders the picker as a status bar replacement.
func renderPicker(items []excludePickerItem, width int) string {
	prefix := lipgloss.NewStyle().Bold(true).Foreground(colorAccentViolet).Render("EXCLUDE ▸ ")

	var parts []string
	for i, item := range items {
		num := lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccentCyan).
			Render(fmt.Sprintf("%d", i+1))
		parts = append(parts, num+": "+stylePrimary.Render(item.label))
	}

	esc := styleMuted.Render("esc: cancel")
	inner := prefix + strings.Join(parts, "   ") + "   " + esc

	return lipgloss.NewStyle().
		PaddingLeft(1).
		Width(width).
		Render(inner)
}

// ── Exclusion Manager Overlay ──

type exclusionManagerModel struct {
	active    bool
	items     []exclusionItem
	cursor    int
	addMode   bool
	addType   int // 0=topic, 1=owner, 2=keyword
	addInput  textinput.Model
	width     int
	height    int
}

type exclusionItem struct {
	kind  string
	value string
}

var addTypeLabels = []string{"topic", "owner", "keyword"}

func newExclusionManager() exclusionManagerModel {
	ti := textinput.New()
	ti.Placeholder = "value…"
	ti.Prompt = "  topic: "
	ti.CharLimit = 80
	return exclusionManagerModel{addInput: ti}
}

func (m *exclusionManagerModel) IsActive() bool { return m.active }

func (m *exclusionManagerModel) Show(set *store.ExclusionSet) {
	m.active = true
	m.cursor = 0
	m.addMode = false
	m.rebuildItems(set)
}

func (m *exclusionManagerModel) Hide() {
	m.active = false
	m.addMode = false
	m.addInput.Blur()
}

func (m *exclusionManagerModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m *exclusionManagerModel) rebuildItems(set *store.ExclusionSet) {
	m.items = nil
	if set == nil {
		return
	}
	for _, v := range set.Topics {
		m.items = append(m.items, exclusionItem{kind: "topic", value: v})
	}
	for _, v := range set.Owners {
		m.items = append(m.items, exclusionItem{kind: "owner", value: v})
	}
	for _, v := range set.Keywords {
		m.items = append(m.items, exclusionItem{kind: "keyword", value: v})
	}
	if m.cursor >= len(m.items) {
		m.cursor = max(0, len(m.items)-1)
	}
}

func (m *exclusionManagerModel) selectedItem() *exclusionItem {
	if len(m.items) == 0 || m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	return &m.items[m.cursor]
}

func (m *exclusionManagerModel) View() string {
	if !m.active {
		return ""
	}

	boxW := m.width * 55 / 100
	if boxW > 70 {
		boxW = 70
	}
	if boxW < 40 {
		boxW = 40
	}
	innerW := boxW - 4

	title := lipgloss.NewStyle().Bold(true).Foreground(colorAccentViolet).Render("◎ Exclusions")
	subtitle := styleMuted.Render(fmt.Sprintf("  %d active", len(m.items)))

	var sections []string
	sections = append(sections, title+subtitle)
	sections = append(sections, "")

	// Render items grouped by kind, with a flat cursor across all items
	for _, kind := range []string{"topic", "owner", "keyword"} {
		header := lipgloss.NewStyle().Bold(true).Foreground(colorAccentViolet).
			Render(strings.ToUpper(kind) + "S")

		var entries []string
		for i, item := range m.items {
			if item.kind != kind {
				continue
			}

			var rendered string
			switch kind {
			case "topic":
				rendered = styleTopicInline.Render(item.value)
			case "owner":
				rendered = styleCyan.Render(item.value)
			case "keyword":
				rendered = stylePrimary.Render("\"" + item.value + "\"")
			}

			if i == m.cursor {
				rendered = lipgloss.NewStyle().
					Background(colorBgElevated).
					Render("▌ " + rendered)
			} else {
				rendered = "  " + rendered
			}
			entries = append(entries, rendered)
		}

		if len(entries) == 0 {
			entries = append(entries, "  "+styleMuted.Render("none"))
		}

		sections = append(sections, "  "+header)
		sections = append(sections, strings.Join(entries, "\n"))
		sections = append(sections, "")
	}

	// Add mode input
	if m.addMode {
		typeLabel := lipgloss.NewStyle().Bold(true).Foreground(colorAccentCyan).
			Render("[" + addTypeLabels[m.addType] + "]")
		sections = append(sections, "  Add "+typeLabel+" "+m.addInput.View())
		sections = append(sections, "  "+styleMuted.Render("tab: cycle type  enter: add  esc: cancel"))
	} else {
		sections = append(sections, styleMuted.Render("  j/k: select  d: remove  a: add  esc: close"))
	}

	content := strings.Join(sections, "\n")

	// Wrap content to innerW
	box := lipgloss.NewStyle().
		Width(innerW).
		Render(content)

	bordered := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentViolet).
		Background(colorBgSurface).
		Padding(1, 2).
		Width(boxW).
		Render(box)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, bordered)
}

