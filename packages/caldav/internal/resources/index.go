package resources

import (
	"strings"
	"sync"
	"unicode"

	"github.com/amarbel-llc/bob/packages/caldav/internal/caldav"
)

// WordIndex is a thread-safe inverted index that maps words to task UIDs.
type WordIndex struct {
	mu    sync.RWMutex
	index map[string][]string // word → []uid
}

// NewWordIndex creates an empty word index.
func NewWordIndex() *WordIndex {
	return &WordIndex{
		index: make(map[string][]string),
	}
}

// IndexItem is a generic item for word indexing.
type IndexItem struct {
	UID  string
	Text string
}

// BuildFromItems rebuilds the index from generic items.
func (idx *WordIndex) BuildFromItems(items []IndexItem) {
	newIndex := make(map[string][]string)
	seen := make(map[string]map[string]bool)

	for _, item := range items {
		words := extractWords(item.Text)
		for _, w := range words {
			if seen[w] == nil {
				seen[w] = make(map[string]bool)
			}
			if !seen[w][item.UID] {
				seen[w][item.UID] = true
				newIndex[w] = append(newIndex[w], item.UID)
			}
		}
	}

	idx.mu.Lock()
	idx.index = newIndex
	idx.mu.Unlock()
}

// Build rebuilds the index from a list of tasks.
func (idx *WordIndex) Build(tasks []caldav.Task) {
	items := make([]IndexItem, len(tasks))
	for i, t := range tasks {
		text := t.Summary + " " + t.Description
		if len(t.Categories) > 0 {
			text += " " + strings.Join(t.Categories, " ")
		}
		items[i] = IndexItem{UID: t.UID, Text: text}
	}
	idx.BuildFromItems(items)
}

// Search returns task UIDs matching the given word.
func (idx *WordIndex) Search(word string) []string {
	w := strings.ToLower(strings.TrimSpace(word))
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Exact match first
	if uids, ok := idx.index[w]; ok {
		return uids
	}

	// Prefix match
	var result []string
	for k, uids := range idx.index {
		if strings.HasPrefix(k, w) {
			result = append(result, uids...)
		}
	}
	return dedupe(result)
}

var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true,
	"at": true, "be": true, "by": true, "for": true, "from": true,
	"has": true, "he": true, "in": true, "is": true, "it": true,
	"its": true, "of": true, "on": true, "or": true, "that": true,
	"the": true, "to": true, "was": true, "were": true, "will": true,
	"with": true,
}

func extractWords(text string) []string {
	var words []string
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(word) >= 3 && !stopWords[word] {
			words = append(words, word)
		}
	}
	return words
}

func dedupe(uids []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, uid := range uids {
		if !seen[uid] {
			seen[uid] = true
			result = append(result, uid)
		}
	}
	return result
}
