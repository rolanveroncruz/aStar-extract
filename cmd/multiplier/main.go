package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	// Update this to your actual sqlc db package path
	"github.com/rolanveroncruz/aStar/extract/internal/db"
	"google.golang.org/genai"
)

/* =================================================================
   Data Structures & JSON Schema
   ================================================================= */

// DerivedQuestion models the expected structured output from gemini-2.5-flash
type DerivedQuestion struct {
	Difficulty    string   `json:"difficulty"`
	QuestionText  string   `json:"question_text"`
	Choices       []string `json:"choices"`
	CorrectChoice string   `json:"correct_choice"`
	Explanation   string   `json:"explanation"`
}

type GeneratorResponse struct {
	IsDerivable      bool              `json:"is_derivable"`
	DerivationIssue  string            `json:"derivation_issue"`
	ExtractedSkills  string            `json:"extracted_skills"`
	DerivedQuestions []DerivedQuestion `json:"derived_questions"`
}

type Job struct {
	ID           int64
	SubjectID    int64
	QuestionText string
}

func main() {
	//godotenv.Load() loads and injects keys not already defined by env.
	// gotdotenv.Overload() loads and overwrites env definitions.
	if err := godotenv.Overload(); err != nil {
		log.Println("Note: No .env file found, relying on environment settings")
	}

	ctx := context.Background()

	// 1. Load the specific Multiplier System Prompt
	promptPath := "prompts/v3_derived_generation.txt" // Or set via env var
	systemPromptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		log.Fatalf("Failed to read system prompt file at %s: %v", promptPath, err)
	}
	systemPrompt := string(systemPromptBytes)

	// 2. Establish PostgreSQL Pool
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Database connection pool initialization failed: %v", err)
	}
	defer pool.Close()
	queries := db.New(pool)

	// 3. Initialize Gemini Client
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY environment variable is not set")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatalf("Failed to initialize Gemini client: %v", err)
	}

	// 4. Fetch the backlog for Math (1), Science (186), and Language Proficiency (113)
	targetSubjects := []int64{1, 186, 3}
	var backlog []Job

	// For each of the subjects, we get 75 questions.

	for _, sid := range targetSubjects {
		qs, err := queries.GetPendingQuestions(ctx, sid)
		if err != nil {
			log.Printf("Failed to fetch pending questions for subject %d: %v", sid, err)
			continue
		}
		for _, q := range qs {
			backlog = append(backlog, Job{
				ID:           q.ID,
				SubjectID:    sid,
				QuestionText: q.QuestionText,
			})
		}
	}

	totalQuestions := len(backlog)
	if totalQuestions == 0 {
		fmt.Println("✅ No un-derived questions found in the targeted subjects. Exiting.")
		return
	}
	fmt.Printf("🔍 Multiplier Engine active. Found %d questions waiting to be derived.\n", totalQuestions)

	startTime := time.Now()

	/* =================================================================
	   Worker Pool Initialization
	   ================================================================= */
	var completedCount int64 = 0
	numWorkers := 3 // Pacing
	jobs := make(chan Job, totalQuestions)
	var wg sync.WaitGroup

	// Seed the pipeline
	for _, job := range backlog {
		jobs <- job
	}
	close(jobs)

	fmt.Printf("🚀 Spawning %d parallel multiplier workers...\n", numWorkers)

	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for job := range jobs {
				// Thread-Safe ETA Calculation
				currentCompleted := atomic.AddInt64(&completedCount, 1)
				var etaDisplay string
				if currentCompleted > 1 {
					elapsed := time.Since(startTime)
					avgTimePerQuestion := elapsed / time.Duration(currentCompleted-1)
					remainingQuestions := int(totalQuestions) - int(currentCompleted)
					etaDisplay = fmt.Sprintf("ETA: %v", (avgTimePerQuestion * time.Duration(remainingQuestions)).Round(time.Second))
				} else {
					etaDisplay = "ETA: calculating..."
				}

				// Fetch original choices for this question
				choicesText, err := fetchChoicesString(ctx, queries, job.ID)
				if err != nil {
					fmt.Printf("[%d/%d] ❌ Worker %d - Failed to fetch choices for QID %d: %v\n", currentCompleted, totalQuestions, workerID, job.ID, err)
					continue
				}

				/* =================================================================
				   Retry Loop with Automated Daily Quota Back-off
				   ================================================================= */
				var result *GeneratorResponse
				var evalErr error

				for {
					result, evalErr = generateDerivativesWithPro(ctx, client, systemPrompt, job.QuestionText, choicesText)
					if evalErr != nil {
						if strings.Contains(evalErr.Error(), "RESOURCE_EXHAUSTED") {
							waitTime := 12 * time.Hour
							fmt.Printf("[%d/%d] ⚠️ Quota Exhausted! Worker %d macro-sleep for %v...\n",
								currentCompleted, totalQuestions, workerID, waitTime)
							time.Sleep(waitTime)
							continue
						}
						fmt.Printf("[%d/%d] ❌ Worker %d - QID %d Error: %v\n", currentCompleted, totalQuestions, workerID, job.ID, evalErr)
						time.Sleep(2 * time.Second)
						break
					}
					break
				}

				if evalErr != nil {
					time.Sleep(10 * time.Second) // Maintain pacing even on failure
					continue
				}

				/* =================================================================
				   Database Routing & Transaction Management
				   ================================================================= */
				if !result.IsDerivable {
					fmt.Printf("[%d/%d] ID %d | %s | Marked Un-derivable | Worker %d\n", currentCompleted, totalQuestions, job.ID, etaDisplay, workerID)
					_ = queries.MarkQuestionUnderivable(ctx, db.MarkQuestionUnderivableParams{
						DerivationIssue: pgtype.Text{String: result.DerivationIssue, Valid: true},
						ID:              job.ID,
					})
				} else {
					err = saveDerivatives(ctx, pool, queries, job, result)
					if err != nil {
						fmt.Printf("[%d/%d] ❌ Worker %d - DB Save Failed for QID %d: %v\n", currentCompleted, totalQuestions, workerID, job.ID, err)
					} else {
						fmt.Printf("[%d/%d] ID %d | %s | %d Variants Created | Worker %d\n",
							currentCompleted, totalQuestions, job.ID, etaDisplay, len(result.DerivedQuestions), workerID)
					}
				}

				// Free Tier Pacing (12 RPM)
				time.Sleep(10 * time.Second)
			}
		}(w)
	}

	wg.Wait()
	fmt.Println("🎉 Derivative question generation complete!")
}

/*
=================================================================

	Database Helpers
	=================================================================
*/
func fetchChoicesString(ctx context.Context, queries *db.Queries, qID int64) (string, error) {
	choices, err := queries.GetChoicesForQuestion(ctx, qID)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, c := range choices {
		// Assuming your sqlc struct for choices returns these
		sb.WriteString(fmt.Sprintf("%s) %s\n", c.ChoiceLetter, c.ChoiceText))
	}
	return sb.String(), nil
}

func saveDerivatives(ctx context.Context, pool *pgxpool.Pool, queries *db.Queries, job Job, response *GeneratorResponse) error {
	// 1. Begin a pgx transaction
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func(tx pgx.Tx, ctx context.Context) {
		err := tx.Rollback(ctx)
		if err != nil {

		}
	}(tx, ctx)

	// 2. Bind queries to the transaction
	qtx := queries.WithTx(tx)

	for _, dq := range response.DerivedQuestions {
		levelID := mapDifficultyToLevelID(dq.Difficulty)

		newQID, err := qtx.InsertDerivedQuestion(ctx, db.InsertDerivedQuestionParams{
			OriginalQuestionID: pgtype.Int8{Int64: job.ID, Valid: true},
			SubjectID:          pgtype.Int8{Int64: job.SubjectID, Valid: true},
			LevelType:          pgtype.Int8{Int64: int64(levelID), Valid: true},
			SkillsTested:       pgtype.Text{String: response.ExtractedSkills, Valid: true},
			QuestionText:       dq.QuestionText,
			CorrectChoice:      dq.CorrectChoice,
			Explanation:        dq.Explanation,
		})
		if err != nil {
			return fmt.Errorf("failed to insert derived question: %w", err)
		}

		letters := []string{"A", "B", "C", "D"}
		for i, choiceText := range dq.Choices {
			if i > 3 {
				break
			}
			err = qtx.InsertDerivedChoice(ctx, db.InsertDerivedChoiceParams{
				QuestionID:   newQID,
				ChoiceText:   choiceText,
				ChoiceLetter: letters[i],
			})
			if err != nil {
				return fmt.Errorf("failed to insert derived choice: %w", err)
			}
		}
	}
	// 3. Commit
	return tx.Commit(ctx)
}

func mapDifficultyToLevelID(difficulty string) int {
	switch strings.ToLower(difficulty) {
	case "easy":
		return 1
	case "medium":
		return 2
	case "hard":
		return 3
	default:
		return 2
	}
}

/*
=================================================================

	Gemini Call Wrapper (Strictly configured for cost savings)
	=================================================================
*/
func generateDerivativesWithPro(ctx context.Context, client *genai.Client, systemPrompt string, questionText string, choicesText string) (*GeneratorResponse, error) {
	// We define the schema exactly as we discussed
	derivativeSchema := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"is_derivable":     {Type: genai.TypeBoolean},
			"derivation_issue": {Type: genai.TypeString},
			"extracted_skills": {Type: genai.TypeString},
			"derived_questions": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"difficulty":     {Type: genai.TypeString},
						"question_text":  {Type: genai.TypeString},
						"correct_choice": {Type: genai.TypeString},
						"explanation":    {Type: genai.TypeString},
						"choices": {
							Type:  genai.TypeArray,
							Items: &genai.Schema{Type: genai.TypeString},
						},
					},
					Required: []string{"difficulty", "question_text", "correct_choice", "explanation", "choices"},
				},
			},
		},
		Required: []string{"is_derivable", "derivation_issue", "extracted_skills", "derived_questions"},
	}

	config := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		Temperature:      genai.Ptr(float32(0.2)),
		ResponseSchema:   derivativeSchema,
		// COST SAVER: System instructions are placed here, NOT in the user prompt array
		SystemInstruction: &genai.Content{
			Role: "system",
			Parts: []*genai.Part{
				{Text: systemPrompt},
			},
		},
	}

	// This tight, tiny payload is the only thing processed dynamically
	userPayload := fmt.Sprintf("Question Text:\n%s\n\nChoices:\n%s", questionText, choicesText)

	resp, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash",
		[]*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					{Text: userPayload},
				},
			},
		},
		config,
	)
	if err != nil {
		return nil, fmt.Errorf("api invocation error: %w", err)
	}

	var res GeneratorResponse
	if err := json.Unmarshal([]byte(resp.Text()), &res); err != nil {
		return nil, fmt.Errorf("failed to parse structured output: %w", err)
	}

	return &res, nil
}
