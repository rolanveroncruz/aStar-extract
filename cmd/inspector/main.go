package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"        // ✅ Added for synchronization
	"sync/atomic" // ✅ Added for thread-safe progress incrementing
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/rolanveroncruz/aStar/extract/internal/db"
	"google.golang.org/genai"
)

// SolverResponse models the expected structured output from gemini-2.5-pro
type SolverResponse struct {
	IsSolvable      bool    `json:"is_solvable"`
	SolvedChoice    string  `json:"solved_choice"`
	ConfidenceScore float64 `json:"confidence_score"`
	Explanation     string  `json:"explanation"`
}

func main() {
	if err := godotenv.Overload(); err != nil {
		log.Println("Note: No .env file found or overridden, relying on environment settings")
	}

	ctx := context.Background()

	promptPath := os.Getenv("VERIFY_PROMPT_FILE")
	if promptPath == "" {
		log.Fatal("VERIFY_PROMPT_FILE environment variable is not set")
	}

	systemInstructionsBytes, err := os.ReadFile(promptPath)
	if err != nil {
		log.Fatalf("Failed to read system prompt file at %s: %v", promptPath, err)
	}
	systemInstructions := string(systemInstructionsBytes)

	// 1. Establish the high-performance connection pool to PostgreSQL
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Database connection pool initialization failed: %v", err)
	}
	defer pool.Close()
	queries := db.New(pool)

	// 2. Initialize the modern Google GenAI SDK Client
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatalf("Failed to initialize Google GenAI client: %v", err)
	}

	// 3. Fetch the backlog of unverified questions
	backlog, err := queries.GetUnverifiedQuestions(ctx)
	if err != nil {
		log.Fatalf("Failed to fetch unverified questions backlog: %v", err)
	}
	totalQuestions := len(backlog)
	fmt.Printf("🔍 Inspector Engine active. Found %d questions waiting to be verified.\n", totalQuestions)

	startTime := time.Now()

	/* ✅ =================================================================
	   ✅ NEW: Worker Pool Initialization Infrastructure
	   ✅ ================================================================= */
	var completedCount int64 = 0                                    // ✅ Thread-safe completion index
	numWorkers := 3                                                 // ✅ Conservative safety throttle against 429 errors
	jobs := make(chan db.GetUnverifiedQuestionsRow, totalQuestions) // ✅ Channel to distribute items
	var wg sync.WaitGroup                                           // ✅ Controls smooth master routine completion block

	// ✅ Seed the jobs pipeline channel buffer completely upfront
	for _, q := range backlog { // ✅
		jobs <- q // ✅
	} // ✅
	close(jobs) // ✅ Secure channel input state

	fmt.Printf("🚀 Spawning %d parallel inspector workers...\n", numWorkers) // ✅

	// ✅ Spawn worker threads to consume the queue concurrently
	for w := 1; w <= numWorkers; w++ { // ✅
		wg.Add(1)               // ✅
		go func(workerID int) { // ✅
			defer wg.Done() // ✅

			// Workers sequentially consume available rows off the shared channel buffer
			for q := range jobs { // ✅
				/* ✅ =================================================================
				   ✅ NEW: Dynamic Thread-Safe ETA Calculation
				   ✅ ================================================================= */
				currentCompleted := atomic.AddInt64(&completedCount, 1) // ✅ Safely fetch current step index

				var etaDisplay string     // ✅
				if currentCompleted > 1 { // ✅
					elapsed := time.Since(startTime)                                  // ✅
					avgTimePerQuestion := elapsed / time.Duration(currentCompleted-1) // ✅
					remainingQuestions := int(totalQuestions) - int(currentCompleted) // ✅
					eta := avgTimePerQuestion * time.Duration(remainingQuestions)     // ✅
					etaDisplay = fmt.Sprintf("ETA: %v", eta.Round(time.Second))       // ✅
				} else { // ✅
					etaDisplay = "ETA: calculating..." // ✅
				} // ✅

				evaluationPayload := fmt.Sprintf("Question Text:\n%s\n\nChoices:\n%s", q.QuestionText, string(q.Choices))

				// Execute the heavy reasoning task via Gemini API
				result, err := evaluateQuestionWithPro(ctx, client, systemInstructions, evaluationPayload)
				if err != nil {
					// ✅ Clean unified logging prevents interleaved console text corruption
					fmt.Printf("[%d/%d] ❌ Worker %d - Question ID %d Error: %v\n", currentCompleted, totalQuestions, workerID, q.ID, err) // ✅
					time.Sleep(2 * time.Second)
					continue
				}

				var confNumeric pgtype.Numeric
				if err := confNumeric.Scan(fmt.Sprintf("%.3f", result.ConfidenceScore)); err != nil {
					fmt.Printf("[%d/%d] ❌ Worker %d - Precision Failure: %v\n", currentCompleted, totalQuestions, workerID, err) // ✅
					continue
				}

				finalChoice := result.SolvedChoice
				if !result.IsSolvable {
					finalChoice = ""
				}

				err = queries.UpdateQuestionVerification(ctx, db.UpdateQuestionVerificationParams{
					ID:              q.ID,
					CorrectChoice:   finalChoice,
					Explanation:     result.Explanation,
					IsSolvable:      pgtype.Bool{Bool: result.IsSolvable, Valid: true},
					ConfidenceScore: confNumeric,
				})

				if err != nil {
					fmt.Printf("[%d/%d] ❌ Worker %d - DB Save Failed: %v\n", currentCompleted, totalQuestions, workerID, err) // ✅
				} else {
					// ✅ Clean unified success line output includes the live ETA metric
					fmt.Printf("[%d/%d] ID %d | %s | Solved: %s | Solvable: %t | Conf: %.2f | Worker %d\n", // ✅
						currentCompleted, totalQuestions, q.ID, etaDisplay, finalChoice, result.IsSolvable, result.ConfidenceScore, workerID) // ✅
				}

				// ✅ Removed the heavy 5-second sleep block.
				// ✅ Natural API latency of gemini-2.5-pro across 3 workers provides safe rate pacing.
			}
		}(w) // ✅
	} // ✅

	wg.Wait() // ✅ Block main thread execution context until all parallel channels wrap up cleanly
	fmt.Println("🎉 Verification backlog processing complete!")
}

// evaluateQuestionWithPro interacts with the reasoning endpoint to solve the exam data
func evaluateQuestionWithPro(ctx context.Context, client *genai.Client, systemInstructions string, questionText string) (*SolverResponse, error) {
	config := &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		Temperature:      genai.Ptr(float32(0.0)),
		ResponseSchema: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"is_solvable": {
					Type:        genai.TypeBoolean,
					Description: "Set to false ONLY if a missing diagram makes calculation completely impossible.",
				},
				"solved_choice": {
					Type:        genai.TypeString,
					Description: "The single letter (A, B, C, D) corresponding to the correct answer. Empty string if unsolvable.",
				},
				"confidence_score": {
					Type:        genai.TypeNumber,
					Description: "Your confidence calculation index value ranging precisely from 0.000 to 1.000.",
				},
				"explanation": {
					Type:        genai.TypeString,
					Description: "Comprehensive, step-by-step mathematical or logical analysis of the solution process.",
				},
			},
			Required: []string{"is_solvable", "solved_choice", "confidence_score", "explanation"},
		},
	}

	resp, err := client.Models.GenerateContent(ctx, "gemini-2.5-pro",
		[]*genai.Content{
			&genai.Content{
				Role: "user",
				Parts: []*genai.Part{
					&genai.Part{Text: systemInstructions + "\n\nQuestion Asset to Evaluate:\n" + questionText},
				},
			},
		},
		config,
	)
	if err != nil {
		return nil, fmt.Errorf("gemini backend invocation error: %w", err)
	}

	var res SolverResponse
	if err := json.Unmarshal([]byte(resp.Text()), &res); err != nil {
		return nil, fmt.Errorf("failed to parse structured solver JSON output: %w", err)
	}

	return &res, nil
}
