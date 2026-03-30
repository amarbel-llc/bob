package git

import (
	"fmt"
	"strings"
)

const StashListFormat = "%gd\x1f%gs\x1e"

func ParseStashList(output string) []StashEntry {
	var entries []StashEntry

	records := strings.Split(strings.TrimSpace(output), branchRecordSep)
	for _, record := range records {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		fields := strings.Split(record, fieldSep)
		if len(fields) < 2 {
			continue
		}

		ref := strings.TrimSpace(fields[0])
		message := strings.TrimSpace(fields[1])

		index := parseStashIndex(ref)

		entries = append(entries, StashEntry{
			Index:   index,
			Message: message,
		})
	}

	if entries == nil {
		entries = []StashEntry{}
	}

	return entries
}

func parseStashIndex(ref string) int {
	var index int
	fmt.Sscanf(ref, "stash@{%d}", &index)
	return index
}
