package seal

import (
	"bytes"
	"fmt"
	"time"

	"github.com/lemon4ksan/przm"
)

// GenerateLLMWeights simulates a BF16/INT8 weight matrix with low-rank structure + sparse noise.
func GenerateLLMWeights(rows, cols, rank int) []byte {
	// We'll generate a block of size rows * cols.
	matrix := make([]byte, rows*cols)

	for i := 0; i < rows; i++ {
		for j := 0; j < cols; j++ {
			matrix[i*cols+j] = byte(i * 13)
		}
	}

	// Inject sparse noise
	for i := 0; i < rows*cols; i += 100 {
		matrix[i] ^= 0xFF
	}

	return matrix
}

// GenerateJSONLogs simulates a repetitive JSON log stream.
func GenerateJSONLogs(count int) []byte {
	var buf bytes.Buffer
	for i := 0; i < count; i++ {
		buf.WriteString(fmt.Sprintf(`{"timestamp": "2026-09-13T22:30:%02dZ", "level": "INFO", "service": "auth-service", "message": "User login successful", "user_id": 100%d, "ip_address": "192.168.1.%d"}`+"\n", i%60, i%10, i%255))
	}
	return buf.Bytes()
}

func main() {
	fmt.Println("=== PRZM Real Data Compression Demo ===")

	// Need a 32KiB dictionary for GlobalLRGGF / CompressTTRN
	dict := make([]byte, 32768)
	// Fill dict with some arbitrary bytes so it's not all zeros (optional but realistic)
	for i := range dict {
		dict[i] = byte(i % 256)
	}

	// 1. LLM Weight Matrix (TTRN)
	fmt.Println("\n--- LLM Weight Matrix (Low-Rank + Sparse Noise) ---")
	llmData := GenerateLLMWeights(256, 256, 4) 
	fmt.Printf("Original Size:   %d bytes\n", len(llmData))
	
	start := time.Now()
	compressedLLM, err := przm.CompressWithDict(llmData, 6, dict)
	duration := time.Since(start)
	
	if err != nil {
		fmt.Printf("Compression failed: %v\n", err)
	} else {
		fmt.Printf("Compressed Size: %d bytes\n", len(compressedLLM))
		fmt.Printf("Ratio:           %.2f%%\n", float64(len(compressedLLM))/float64(len(llmData))*100)
		fmt.Printf("Execution Time:  %v\n", duration)
	}

	// 2. JSON Log Stream (Global LRGGF)
	fmt.Println("\n--- Repetitive JSON Log Stream ---")
	jsonData := GenerateJSONLogs(500)
	fmt.Printf("Original Size:   %d bytes\n", len(jsonData))
	
	start = time.Now()
	compressedJSON, err := przm.CompressWithDict(jsonData, 6, dict)
	duration = time.Since(start)
	
	if err != nil {
		fmt.Printf("Compression failed: %v\n", err)
	} else {
		fmt.Printf("Compressed Size: %d bytes\n", len(compressedJSON))
		fmt.Printf("Ratio:           %.2f%%\n", float64(len(compressedJSON))/float64(len(jsonData))*100)
		fmt.Printf("Execution Time:  %v\n", duration)
	}
}
