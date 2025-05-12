package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

const (
	inputFile       = `saol_entries.json`
	outputFile      = "flattened_lemmas.json"
	numWorkers      = 0
	channelBufferSize = 100
)


type InputEntry struct {
	HTML string `json:"html"`
}

type Job struct {
	Index int
	Data  InputEntry
}

type Result struct {
	Index      int
	LemmaHTMLs []string
	Error      error
}

type LemmaOutput struct {
	HTML     string `json:"html"`
	FamilyID int    `json:"familyID"`
}

func main() {
	log.Println("Starting JSON HTML processing for flattened lemmas...")

	workers := numWorkers
	if workers <= 0 {
		workers = runtime.NumCPU()
		if workers < 1 {
			workers = 1
		}
	}
	log.Printf("Using %d worker goroutines", workers)

	file, err := os.Open(inputFile)
	if err != nil {
		log.Fatalf("Error opening input file '%s'. Error: %v", inputFile, err)
	}
	defer file.Close()

	outFile, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("Error creating output file '%s': %v", outputFile, err)
	}
	defer outFile.Close()

	jobs := make(chan Job, channelBufferSize)
	results := make(chan Result, channelBufferSize)
	var wg sync.WaitGroup

	log.Println("Launching workers...")
	for w := 1; w <= workers; w++ {
		wg.Add(1)
		go worker(w, jobs, results, &wg)
	}

	var collectorWg sync.WaitGroup
	collectedResults := make([]Result, 0)
	collectorWg.Add(1)
	go func() {
		defer collectorWg.Done()
		for res := range results {
			if res.Error != nil {
				log.Printf("Worker Error (Original Index %d): %v. Skipping this entry.", res.Index, res.Error)
				continue
			}
			collectedResults = append(collectedResults, res)
		}
		log.Println("Result collection finished.")
	}()

	log.Println("Reading input JSON and dispatching jobs...")
	decoder := json.NewDecoder(file)
	token, err := decoder.Token()
	if err != nil {
		log.Fatalf("Error reading initial JSON token: %v", err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		log.Fatalf("Expected JSON array start '[', but got: %T %v", token, token)
	}

	index := 0
	for decoder.More() {
		var entry InputEntry
		err := decoder.Decode(&entry)
		if err != nil {
			if err == io.EOF {
				log.Println("Reached end of JSON stream unexpectedly inside array.")
				break
			}
			log.Printf("Error decoding JSON object at index %d: %v. Skipping.", index, err)
			var raw json.RawMessage
			_ = decoder.Decode(&raw)
			index++
			continue
		}
		jobs <- Job{Index: index, Data: entry}
		index++
	}

	token, err = decoder.Token()
	if err != nil && err != io.EOF {
		log.Printf("Warning: Error reading final JSON token: %v", err)
	} else if delim, ok := token.(json.Delim); ok && delim == ']' {
		log.Println("Finished reading JSON array.")
	} else if token != nil {
		log.Printf("Warning: Expected JSON array end ']', but got: %T %v", token, token)
	}

	close(jobs)
	log.Println("All jobs dispatched. Waiting for workers...")

	wg.Wait()
	log.Println("All workers finished.")

	close(results)
	log.Println("Results channel closed. Waiting for collector...")

	collectorWg.Wait()
	log.Println("Collector finished.")

	log.Println("Processing collected results into final format...")

	sort.Slice(collectedResults, func(i, j int) bool {
		return collectedResults[i].Index < collectedResults[j].Index
	})

	finalOutput := make(map[int]LemmaOutput)
	outputKey := 1
	totalLemmasProcessed := 0
	for _, res := range collectedResults {
		familyID := res.Index + 1
		for _, lemmaHTML := range res.LemmaHTMLs {
			entry := LemmaOutput{
				HTML:     lemmaHTML,
				FamilyID: familyID,
			}
			finalOutput[outputKey] = entry
			outputKey++
			totalLemmasProcessed++
		}
	}
	log.Printf("Prepared final map with %d individual lemma entries.", totalLemmasProcessed)

	log.Println("Writing output JSON file...")
	encoder := json.NewEncoder(outFile)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(finalOutput)
	if err != nil {
		log.Fatalf("Error encoding final JSON output: %v", err)
	}

	log.Printf("Successfully processed %d original entries resulting in %d lemma entries, saved to '%s'.", len(collectedResults), totalLemmasProcessed, outputFile)
}

func worker(id int, jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()

	for job := range jobs {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(job.Data.HTML))
		if err != nil {
			results <- Result{Index: job.Index, Error: fmt.Errorf("failed to parse HTML: %w", err)}
			continue
		}

		articleSelection := doc.Find("div.article")
		if articleSelection.Length() == 0 {
			results <- Result{Index: job.Index, LemmaHTMLs: []string{}}
			continue
		}

		lemmaSelection := articleSelection.First().Find("div.lemma")
		lemmasHTML := make([]string, 0, lemmaSelection.Length())

	
		lemmaSelection.Each(func(i int, s *goquery.Selection) { 
			html, err := s.Html()
			if err != nil {
				log.Printf("Worker %d: Error getting HTML for a lemma within original index %d: %v. Skipping lemma.", id, job.Index, err)
				return 
			}
			lemmasHTML = append(lemmasHTML, html)
		})

		
		results <- Result{Index: job.Index, LemmaHTMLs: lemmasHTML}
	}
}
