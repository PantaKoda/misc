package main

import (
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

func main() {
	inputFile := "flattened_lemmas.json"

	log.Println("Calling FilterLemmasByOrdklass...")
	filteredHTMLs, err := FilterLemmasByOrdklass(inputFile)
	if err != nil {
		log.Fatalf("Function failed: %v", err)
	}

	log.Printf("Successfully filtered lemmas. Number of matching HTML entries: %d", len(filteredHTMLs))

	log.Println("First few matching HTMLs:")

	nouns := [][]string{}
	verbs := [][]string{}
	adjectives := [][]string{}
	for _, html := range filteredHTMLs {

		reader := strings.NewReader(html)
		doc, err := goquery.NewDocumentFromReader(reader)
		if err != nil {
			log.Fatal(err)
		}

		switch doc.Find(".ordklass").Text() {
		case "substantiv":
			nouns = append(nouns, parseSubstantiv(doc))
		case "verb":
			verbs = append(verbs, parseVerbForms(doc))
		case "adjektiv":
			adjectives = append(adjectives, parseAdjektiv(doc))
		}
	}

	if err := saveAdjectivesJSON(adjectives, "adjectives.json"); err != nil {
		log.Fatalf("Failed to write adjectives.json: %v", err)
	}

	if err := saveVerbsJSON(verbs, "verbs.json"); err != nil {
		log.Fatalf("could not save verbs.json: %v", err)
	}

	for i, verb := range verbs {
		fmt.Printf("%d: %s\n", i+1, strings.Join(verb, "; "))
	}
}

func parseSubstantiv(doc *goquery.Document) []string {
	var nouns []string
	currentCase := ""

	doc.Find(".tabell tr").Each(func(_ int, s *goquery.Selection) {

		if th := s.Find("th.ordformth"); th.Length() == 1 {
			currentCase = strings.TrimSpace(th.Find("i").Text())
			return
		}

		tds := s.Find("td")
		if tds.Length() != 2 {
			return
		}

		nounText := strings.TrimSpace(tds.Eq(0).Text())

		ledText := strings.TrimSpace(tds.Eq(1).Text())
		parts := strings.Fields(ledText)
		var ledWord string
		if len(parts) > 0 {
			ledWord = parts[0]
		}

		entry := fmt.Sprintf("%s-%s-%s", nounText, ledWord, currentCase)
		nouns = append(nouns, entry)
	})

	return nouns
}

// parseVerbForms walks one .tabell and returns a []string where each entry
// is "form-tense voice-Section", e.g. "knäsätter-presens aktiv-Finita former".
func parseVerbForms(doc *goquery.Document) []string {
	var forms []string
	currentSection := ""

	doc.Find(".tabell tr").Each(func(_ int, s *goquery.Selection) {
		if th := s.Find("th.ordformth"); th.Length() == 1 {
			currentSection = strings.TrimSpace(th.Find("i").Text())
			return
		}

		tds := s.Find("td")
		if tds.Length() == 0 {
			return
		}

		formText := strings.TrimSpace(tds.Eq(0).Text())

		var tenseVoice string
		if tds.Length() > 1 {
			tenseVoice = strings.TrimSpace(tds.Eq(1).Text())
		}

		entry := formText
		if tenseVoice != "" {
			entry += "-" + tenseVoice
		}
		entry += "-" + currentSection

		forms = append(forms, entry)
	})

	return forms
}
func saveVerbsJSON(all [][]string, filename string) error {
	type verbJSON struct {
		Class string              `json:"class"`
		Forms map[string][]string `json:"forms"`
	}

	var out []verbJSON

	for _, raw := range all {
		entry := verbJSON{
			Class: "verb",
			Forms: map[string][]string{
				"Finita former":    {},
				"Infinita former":  {},
				"Presens particip": {},
				"Perfekt particip": {},
			},
		}

		for _, tagged := range raw {

			last := strings.LastIndex(tagged, "-")
			if last < 0 {
				continue
			}
			section := tagged[last+1:]
			fv := tagged[:last]
			if _, ok := entry.Forms[section]; ok {
				entry.Forms[section] = append(entry.Forms[section], fv)
			}
		}
		out = append(out, entry)
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filename, data, 0644)
}
func parseAdjektiv(doc *goquery.Document) []string {
	var entries []string
	currentDegree := ""

	doc.Find(".tabell tr").Each(func(_ int, s *goquery.Selection) {

		if th := s.Find("th.ordformth"); th.Length() == 1 {
			currentDegree = strings.TrimSpace(th.Find("i").Text())
			return
		}

		tds := s.Find("td")
		if tds.Length() != 1 {
			return
		}

		raw := strings.TrimSpace(tds.Eq(0).Text())

		parts := strings.SplitN(raw, "+", 2)
		form := strings.TrimSpace(parts[0])

		entry := fmt.Sprintf("%s-%s", form, currentDegree)
		entries = append(entries, entry)
	})

	return entries
}

// AdjectiveEntry defines the JSON schema without an ID.
type AdjectiveEntry struct {
	Class string              `json:"class"`
	Forms map[string][]string `json:"forms"`
}

// saveAdjectivesJSON takes a slice of slice-of-strings and writes the JSON file.
func saveAdjectivesJSON(adjs [][]string, filename string) error {
	// Prepare a slice of entries
	entries := make([]AdjectiveEntry, len(adjs))

	for i, rawForms := range adjs {
		// Initialize with fixed degrees
		entry := AdjectiveEntry{
			Class: "adjektiv",
			Forms: map[string][]string{
				"Positiv":    {},
				"Komparativ": {},
				"Superlativ": {},
			},
		}

		// Populate based on each "form-Degree" string
		for _, tagged := range rawForms {
			// split at the last "-"
			idx := strings.LastIndex(tagged, "-")
			if idx < 0 {
				// malformed entry; skip or log
				continue
			}
			form := tagged[:idx]
			degree := tagged[idx+1:]

			// only append if it's one of the three known degrees
			if _, ok := entry.Forms[degree]; ok {
				entry.Forms[degree] = append(entry.Forms[degree], form)
			}
		}

		entries[i] = entry
	}

	// Marshal to pretty JSON
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	// Write to the given filename
	return ioutil.WriteFile(filename, data, 0644)
}

type LemmaInput struct {
	HTML     string `json:"html"`
	FamilyID int    `json:"familyID"`
}

func FilterLemmasByOrdklass(filename string) ([]string, error) {
	allowedOrdklass := map[string]bool{
		"substantiv": true,
		"verb":       true,
		"adjektiv":   true,
		"adverb":     true,
	}
	ordklassSelector := ".ordklass"

	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("error opening input file '%s': %w", filename, err)
	}
	defer file.Close()

	var inputMap map[string]LemmaInput
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&inputMap)
	if err != nil {
		return nil, fmt.Errorf("error decoding JSON from '%s': %w", filename, err)
	}

	matchingHTMLs := make([]string, 0)

	log.Printf("Processing %d entries from %s...", len(inputMap), filename)
	processedCount := 0
	for key, entry := range inputMap {
		processedCount++
		if processedCount%1000 == 0 {
			log.Printf("...processed %d entries", processedCount)
		}

		reader := strings.NewReader(entry.HTML)
		doc, err := goquery.NewDocumentFromReader(reader)
		if err != nil {

			log.Printf("Warning: Failed to parse HTML for entry key '%s'. Skipping. Error: %v", key, err)
			continue
		}

		ordklassText := strings.TrimSpace(doc.Find(ordklassSelector).First().Text())

		if allowedOrdklass[ordklassText] {

			matchingHTMLs = append(matchingHTMLs, entry.HTML)
		}
	}
	log.Printf("Finished processing. Found %d matching entries.", len(matchingHTMLs))

	return matchingHTMLs, nil
}
