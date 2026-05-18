package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/rolanveroncruz/aStar/extract/internal/db"
	"google.golang.org/genai"
)

// ----------------------------------------------------------------------
// ✅✅✅ DATA STRUCTURES ALIGNED WITH FINALIZED SCHEMA ✅✅✅
// ----------------------------------------------------------------------

// ExtractedData is the structure that holds the extracted data
type ExtractedData struct {
	Groups []QuestionGroup `json:"groups"` //  Corrected tag from "questions" to "groups"
}
type QuestionGroup struct {
	InstructionText string     `json:"instruction_text,omitempty"`
	Questions       []Question `json:"questions"`
}

// Question is the structure that holds the extracted question data
type Question struct {
	RefNo             string   `json:"ref_no"`
	Subject           string   `json:"subject"`
	Topic             string   `json:"topic"`
	ReferencesDiagram bool     `json:"references_diagram"`
	QuestionText      string   `json:"question_text"`
	CorrectChoice     string   `json:"correct_choice"` // Answer key lives here
	Choices           []Choice `json:"choices"`
}

// Choice is the structure that holds the extracted choice data
type Choice struct {
	ChoiceLetter string `json:"choice_letter"`
	ChoiceText   string `json:"choice_text"`
}

func main() {
	startTime := time.Now()

	//Load the .env file at app startup
	if err := godotenv.Overload(); err != nil {
		log.Println("No .env file found")
		os.Exit(1)
	}
	ctx := context.Background()
	dbString := os.Getenv("DATABASE_URL")
	if dbString == "" {
		log.Fatal("DATABASE_URL not set")
	}

	pool, err := pgxpool.New(ctx, dbString)
	if err != nil {
		log.Fatalf("Unable to connect to the database: %v", err)
	}
	defer pool.Close()
	queries := db.New(pool)

	promptPath := os.Getenv("PROMPT_FILE")
	if promptPath == "" {
		log.Fatal("PROMPT_PATH not set in .env")
	}
	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		log.Fatalf("Failed to read prompt file at %s: %v", promptPath, err)
	}
	promptText := string(promptBytes)

	// Initialize Gemini Client
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatalf("Unable to initialize Gemini client: %v", err)
	}

	dataDir := "/home/rolanveroncruz/aStar/reviewer_data"
	var allFiles []string

	err = filepath.WalkDir(dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// if it is a file and ends in .pdf, save its exact path
		if !d.IsDir() && filepath.Ext(d.Name()) == ".pdf" {
			allFiles = append(allFiles, path)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to walk directory: %v", err)
	}

	// ==================================================================
	// 🧪 TEST RUN LIMITER: TAKE ONLY THE FIRST 6 FILES (2 ROUNDS FOR 3 WORKERS)
	// ==================================================================
	/*	if len(allFiles) > 6 {
			fmt.Printf("🔬 Test Mode: Truncating total files from %d down to 6.\n", len(allFiles))
			allFiles = allFiles[:6]
		}
	*/ // ==================================================================
	var failedFiles []string
	var mu sync.Mutex

	// Channel configuration for pipeline coordination
	numWorkers := 10 // Adjust based on API rate limit tier
	jobs := make(chan string, len(allFiles))
	var wg sync.WaitGroup

	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go worker(ctx, client, queries, promptText, jobs, &wg, &failedFiles, &mu)
	}

	//Enqueue all files
	for _, path := range allFiles {
		jobs <- path
	}

	close(jobs)

	wg.Wait()
	endTime := time.Now()
	duration := time.Since(startTime)
	// ==================================================================
	// ✅ FINAL PIPELINE AUDIT REPORT
	// ==================================================================
	fmt.Println("\n==================================================")
	fmt.Println("🎉 Processing complete!")
	fmt.Println("==================================================")
	fmt.Printf("📅 Started at:  %s\n", startTime.Format("2006-01-02 15:04:05 MST")) // ✅
	fmt.Printf("📅 Ended at:    %s\n", endTime.Format("2006-01-02 15:04:05 MST"))   // ✅
	fmt.Printf("⏱️ Total Execution Time: %v\n", duration)                          // ✅
	fmt.Println("==================================================")

	if len(failedFiles) == 0 {
		fmt.Println("✨ Flawless Run! All processed files extracted completely.")
	} else {
		fmt.Printf("❌ The following %d files exceeded output token boundaries:\n", len(failedFiles))
		for _, file := range failedFiles {
			fmt.Printf("  • %s\n", file)
		}
		fmt.Println("\n💡 Next step: Split these specific PDFs and run the tool again.")
	}
	fmt.Println("==================================================")
}

func worker(ctx context.Context, client *genai.Client, queries *db.Queries, promptText string,
	jobs <-chan string, wg *sync.WaitGroup, failedFiles *[]string, mu *sync.Mutex) {
	defer wg.Done()
	for path := range jobs {
		err := processFile(ctx, client, queries, promptText, path, failedFiles, mu)
		if err != nil {
			fmt.Printf("Failed [%s]: %v\n", filepath.Base(path), err)
		}
		time.Sleep(2 * time.Second)
	}
}

func processFile(ctx context.Context, client *genai.Client, queries *db.Queries,
	promptText string, filePath string, failedFiles *[]string, mu *sync.Mutex) error {

	// ==================================================================
	// ✅ FORCE ABSOLUTE PATH TO INSULATE STRUCTURAL MATCHING
	// ==================================================================
	absPath, err := filepath.Abs(filePath) //  Added line
	if err != nil {                        //  Added line
		return fmt.Errorf("failed to get absolute path: %w", err) //  Added line
	}
	fileName := filepath.Base(filePath)

	// ==================================================================
	//  DYNAMIC LEFT-TO-RIGHT PATH EXTRACTOR
	// ==================================================================
	cleanPath := filepath.Clean(absPath)
	parts := strings.Split(cleanPath, string(filepath.Separator))
	motherFolder := "unknown"

	for i, part := range parts {
		if part == "reviewer_data" && i+2 < len(parts) {
			motherFolder = parts[i+2]
			break
		}
	}
	// ==================================================================
	//  FETCH FILE SIZE METADATA
	// ==================================================================
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file stats: %w", err)
	}
	fileSizeBytes := fileInfo.Size()
	// ==================================================================

	//--- 1. Resume Logic: Check or create the source tracker
	source, err := queries.UpsertSource(ctx, db.UpsertSourceParams{
		FileName:      fileName,
		FilePath:      filePath,
		MotherFolder:  motherFolder,
		FileSizeBytes: fileSizeBytes,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert source: %w", err)
	}
	if source.ProcessedEnd.Valid {
		fmt.Printf("Skipping [%s] - already processed.\n", fileName)
		return nil
	}
	//--- 2. Clean up any orphaned questions from a previous crash mid-file
	_ = queries.ClearIncompleteQuestions(ctx, source.ID)

	fmt.Printf("Processing [%s]...\n", fileName)

	//---3. Extract Data via Gemini
	//---3a. Set up retry parameters
	var data *ExtractedData
	maxRetries := 3
	backoff := 4 * time.Second

	for i := 0; i < maxRetries; i++ {
		data, err = extractDataFromGemini(ctx, client, promptText, filePath)
		if err == nil {
			break // Success!
		}
		fmt.Printf("⚠️ Attempt %d failed for [%s]: %v. Retrying in %v...\n", i+1, fileName, err, backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
		}
	}
	if err != nil {
		// ==============================================================
		// ✅ SECURE MUTEX LOCK TO RECORD FAILED FILE SAFELY
		// ==============================================================
		mu.Lock()                                     // ✅
		*failedFiles = append(*failedFiles, fileName) // ✅
		mu.Unlock()                                   // ✅
		return fmt.Errorf("failed to extract data after %d attempts: %w", maxRetries, err)
	}

	// ----4. Persist to Database
	err = saveExtractionToDB(ctx, queries, source.ID, data)
	if err != nil {
		return fmt.Errorf("failed to save extraction to DB: %w", err)
	}

	//---5. Marke as finished so resume logic knows it is complete
	err = queries.MarkSourceCompleted(ctx, source.ID)
	if err == nil {
		fmt.Printf(" ✅ Success [%s] - %d question groups inserted.\n", fileName, len(data.Groups))
	}
	return err

}

// ----------------------------------------------------------------------
// 2. THE EXTRACTOR (Gemini API & JSON Parsing)
// ----------------------------------------------------------------------
func extractDataFromGemini(ctx context.Context, client *genai.Client, promptText string, filePath string) (*ExtractedData, error) {
	// Upload large file to the File API bucket
	uploadedFile, err := client.Files.UploadFromPath(ctx, filePath, nil)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}
	defer func() {
		_, _ = client.Files.Delete(ctx, uploadedFile.Name, nil)
	}()

	config := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		Temperature:      genai.Ptr(float32(0.0)),
		ResponseSchema: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"groups": {
					Type: genai.TypeArray,
					Items: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"instruction_text": {Type: genai.TypeString}, // Empty string if standalone
							"questions": {
								Type: genai.TypeArray,
								Items: &genai.Schema{
									Type: genai.TypeObject,
									Properties: map[string]*genai.Schema{
										"ref_no": {Type: genai.TypeString},
										"subject": {
											Type: genai.TypeString,
											Enum: []string{"Mathematics", "Science", "Language Proficiency", "Reading Comprehension"},
										},
										"topic":              {Type: genai.TypeString},
										"references_diagram": {Type: genai.TypeBoolean},
										"question_text":      {Type: genai.TypeString},
										"correct_choice":     {Type: genai.TypeString},
										"choices": {
											Type: genai.TypeArray,
											Items: &genai.Schema{
												Type: genai.TypeObject,
												Properties: map[string]*genai.Schema{
													"choice_letter": {Type: genai.TypeString},
													"choice_text":   {Type: genai.TypeString},
												},
												Required: []string{"choice_letter", "choice_text"},
											},
										},
									},
									Required: []string{"ref_no", "subject", "topic", "question_text", "correct_choice"},
								},
							},
						},
						Required: []string{"questions"},
					},
				},
			},
			Required: []string{"groups"},
		},
	}

	// ✅✅✅ FULLY STRICT POINTER SYNTAX ✅✅✅
	resp, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash",
		[]*genai.Content{
			&genai.Content{ // Added &
				Role: "user",
				Parts: []*genai.Part{ // Added *
					{FileData: &genai.FileData{FileURI: uploadedFile.URI, MIMEType: uploadedFile.MIMEType}}, // Added &
					{Text: promptText}, // Added &
				},
			},
		},
		config,
	)
	if err != nil {
		return nil, fmt.Errorf("generation request failed: %w", err)
	}

	var data ExtractedData
	if err := json.Unmarshal([]byte(resp.Text()), &data); err != nil {
		return nil, fmt.Errorf("json parse failed: %w", err)
	}

	return &data, nil
}

// ----------------------------------------------------------------------
// 3. THE PERSISTER (Database Insertions)
// ----------------------------------------------------------------------
func saveExtractionToDB(ctx context.Context, queries *db.Queries, sourceID int64, data *ExtractedData) error {
	// 1. Loop through each Question Group extracted by Gemini
	for _, group := range data.Groups {

		// Initialize our context tracking variable
		var instructionContextID pgtype.Int8

		// 2. Check if this group actually has umbrella instructions
		if group.InstructionText != "" {
			ctxID, err := queries.CreateInstructionContext(ctx, db.CreateInstructionContextParams{
				SourceID:    sourceID,
				ContextText: group.InstructionText,
			})
			if err != nil {
				// If creating the instruction header fails, log it and skip the whole group to maintain relational hygiene
				fmt.Printf("⚠️ Failed creating instruction context: %v\n", err)
				continue
			}
			// Safely map our newly minted int64 ID to the pgtype wrapper
			instructionContextID = pgtype.Int8{Int64: ctxID, Valid: true}
		}

		// 3. Process each individual question inside this specific group
		for _, q := range group.Questions {

			// Upsert the subject for THIS specific question
			subjectID, err := queries.UpsertSubject(ctx, q.Subject)
			if err != nil {
				continue // Skip single question if subject assignment fails
			}

			// Upsert the topic for THIS specific question
			topicID, err := queries.UpsertTopic(ctx, db.UpsertTopicParams{
				SubjectID: subjectID,
				Name:      q.Topic,
			})
			if err != nil {
				continue // Skip single question if topic assignment fails
			}

			// Create the question, including our newly added InstructionContextID link
			qID, err := queries.CreateQuestion(ctx, db.CreateQuestionParams{
				SourceID:             sourceID,
				SubjectID:            subjectID,
				TopicID:              pgtype.Int8{Int64: topicID, Valid: true},
				InstructionContextID: instructionContextID, // Automatically NULL if group had no text
				RefNo:                q.RefNo,
				ReferencesDiagram:    q.ReferencesDiagram,
				QuestionText:         q.QuestionText,
				CorrectChoice:        q.CorrectChoice,
			})
			if err != nil {
				continue
			}

			// 4. Insert choices associated with this specific question
			for _, c := range q.Choices {
				_ = queries.CreateChoice(ctx, db.CreateChoiceParams{
					QuestionID:   qID,
					ChoiceText:   c.ChoiceText,
					ChoiceLetter: c.ChoiceLetter,
				})
			}
		}
	}

	return nil
}
