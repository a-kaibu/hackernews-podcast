package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/braheezy/shine-mp3/pkg/mp3"
	"github.com/go-audio/wav"
)

const (
	defaultPodcastTTSTimeout     = 120 * time.Minute
	defaultVoicevoxReadyTimeout  = 2 * time.Minute
	defaultVoicevoxURL           = "http://localhost:50021"
	defaultVoicevoxSpeakerID     = 3 // ずんだもん（ノーマル）
	defaultVoicevoxSpeedScale    = 1.3
	defaultVoicevoxReadyInterval = 2 * time.Second
)

func generatePodcast(date string, items []NewsItem) error {
	if len(items) == 0 {
		return fmt.Errorf("no podcast items found")
	}

	scripts := buildPodcastScripts(date, items)
	audioPath := filepath.Join(getEnv("PODCAST_OUTPUT_DIR", "."), fmt.Sprintf("hackernews_%s_podcast.mp3", date))

	ctx, cancel := context.WithTimeout(context.Background(), defaultPodcastTTSTimeout)
	defer cancel()

	if err := generateSpeech(ctx, scripts, audioPath); err != nil {
		return fmt.Errorf("failed to generate podcast audio: %w", err)
	}

	fmt.Printf("Podcast audio saved: %s\n", audioPath)
	return nil
}

func generateSpeech(ctx context.Context, scripts []string, audioPath string) error {
	if len(scripts) == 0 {
		return fmt.Errorf("no speech chunks found")
	}

	voicevoxURL := strings.TrimRight(getEnv("VOICEVOX_URL", defaultVoicevoxURL), "/")
	speakerID := getEnvInt("VOICEVOX_SPEAKER_ID", defaultVoicevoxSpeakerID)
	speedScale := getEnvFloat("VOICEVOX_SPEED_SCALE", defaultVoicevoxSpeedScale)

	fmt.Printf("Generating podcast with VOICEVOX: %s (speaker=%d, speed=%.1f)\n", voicevoxURL, speakerID, speedScale)

	if err := waitForVoicevox(ctx, voicevoxURL); err != nil {
		return err
	}

	wavDataList := make([][]byte, 0, len(scripts))
	for i, script := range scripts {
		fmt.Printf("Synthesizing chunk %d/%d\n", i+1, len(scripts))

		wavData, err := synthesizeSpeech(ctx, voicevoxURL, script, speakerID, speedScale)
		if err != nil {
			return fmt.Errorf("failed to synthesize chunk %d: %w", i+1, err)
		}
		wavDataList = append(wavDataList, wavData)
	}

	combinedWavData, err := combineWavData(wavDataList)
	if err != nil {
		return fmt.Errorf("failed to combine chunk audio: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(audioPath), 0755); err != nil {
		return fmt.Errorf("failed to create podcast output directory: %w", err)
	}

	if err := convertWavToMp3(combinedWavData, audioPath); err != nil {
		return fmt.Errorf("failed to convert to MP3: %w", err)
	}

	return nil
}

func synthesizeSpeech(ctx context.Context, baseURL, text string, speakerID int, speedScale float64) ([]byte, error) {
	// Step 1: Get audio query
	queryURL := fmt.Sprintf("%s/audio_query?text=%s&speaker=%d", baseURL, url.QueryEscape(text), speakerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create audio_query request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("audio_query request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("audio_query returned status %d: %s", resp.StatusCode, string(body))
	}

	var audioQuery map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&audioQuery); err != nil {
		return nil, fmt.Errorf("failed to decode audio_query response: %w", err)
	}

	// Step 2: Modify speedScale
	audioQuery["speedScale"] = speedScale

	// Step 3: Synthesize with modified query
	queryBody, err := json.Marshal(audioQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal modified audio_query: %w", err)
	}

	synthURL := fmt.Sprintf("%s/synthesis?speaker=%d", baseURL, speakerID)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, synthURL, bytes.NewReader(queryBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create synthesis request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("synthesis request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("synthesis returned status %d: %s", resp.StatusCode, string(body))
	}

	wavData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read synthesis response: %w", err)
	}

	return wavData, nil
}

func waitForVoicevox(ctx context.Context, voicevoxURL string) error {
	readyCtx, cancel := context.WithTimeout(ctx, defaultVoicevoxReadyTimeout)
	defer cancel()

	ticker := time.NewTicker(defaultVoicevoxReadyInterval)
	defer ticker.Stop()

	var lastErr error
	for {
		lastErr = checkVoicevoxReady(readyCtx, voicevoxURL)
		if lastErr == nil {
			return nil
		}

		select {
		case <-readyCtx.Done():
			return fmt.Errorf("VOICEVOX engine was not ready before timeout: %w", lastErr)
		case <-ticker.C:
		}
	}
}

func checkVoicevoxReady(ctx context.Context, voicevoxURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, voicevoxURL+"/version", nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status from VOICEVOX /version: %d", resp.StatusCode)
	}

	return nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f
	}
	return fallback
}

func buildPodcastScripts(date string, items []NewsItem) []string {
	scripts := []string{fmt.Sprintf("Hacker News 日本語まとめ、%s です。", date)}

	for i, item := range items {
		scripts = append(scripts, fmt.Sprintf("%d位 %s。", i+1, normalizePodcastText(item.TitleJa)))
		if item.CommentSummaryHtml != "" {
			summary := normalizePodcastText(item.CommentSummaryHtml)
			if summary != "" {
				scripts = append(scripts, summary)
			}
		}
	}

	scripts = append(scripts, "以上、今日のHacker News日本語まとめでした。気になった記事は本文のリンクから詳しく確認してみてください。")
	return scripts
}

func normalizePodcastText(s string) string {
	s = regexp.MustCompile(`<[^>]*>`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "HN:", "Hacker News:")
	return strings.Join(strings.Fields(s), " ")
}

func combineWavData(wavDataList [][]byte) ([]byte, error) {
	if len(wavDataList) == 0 {
		return nil, fmt.Errorf("no WAV data to combine")
	}
	if len(wavDataList) == 1 {
		return wavDataList[0], nil
	}

	var allSamples []int
	var sampleRate int
	var numChannels int
	var bitDepth int

	for i, wavData := range wavDataList {
		reader := bytes.NewReader(wavData)
		decoder := wav.NewDecoder(reader)

		buf, err := decoder.FullPCMBuffer()
		if err != nil {
			return nil, fmt.Errorf("failed to decode WAV chunk %d: %w", i, err)
		}

		if i == 0 {
			sampleRate = buf.Format.SampleRate
			numChannels = buf.Format.NumChannels
			bitDepth = int(decoder.BitDepth)
		}

		allSamples = append(allSamples, buf.Data...)
	}

	return encodeWav(allSamples, sampleRate, numChannels, bitDepth)
}

func encodeWav(samples []int, sampleRate, numChannels, bitDepth int) ([]byte, error) {
	var buf bytes.Buffer

	bytesPerSample := bitDepth / 8
	dataSize := uint32(len(samples) * bytesPerSample)
	fileSize := 36 + dataSize

	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, fileSize)
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(numChannels))
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	byteRate := uint32(sampleRate * numChannels * bytesPerSample)
	binary.Write(&buf, binary.LittleEndian, byteRate)
	blockAlign := uint16(numChannels * bytesPerSample)
	binary.Write(&buf, binary.LittleEndian, blockAlign)
	binary.Write(&buf, binary.LittleEndian, uint16(bitDepth))

	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, dataSize)

	for _, sample := range samples {
		switch bitDepth {
		case 16:
			binary.Write(&buf, binary.LittleEndian, int16(sample))
		case 24:
			s := int32(sample)
			buf.WriteByte(byte(s))
			buf.WriteByte(byte(s >> 8))
			buf.WriteByte(byte(s >> 16))
		case 32:
			binary.Write(&buf, binary.LittleEndian, int32(sample))
		default:
			binary.Write(&buf, binary.LittleEndian, int16(sample))
		}
	}

	return buf.Bytes(), nil
}

func convertWavToMp3(wavData []byte, outputPath string) error {
	wavReader := bytes.NewReader(wavData)
	wavDecoder := wav.NewDecoder(wavReader)

	wavBuffer, err := wavDecoder.FullPCMBuffer()
	if err != nil {
		return fmt.Errorf("failed to decode WAV data: %w", err)
	}

	var decodedData []int16
	numChannels := wavBuffer.Format.NumChannels

	if numChannels == 1 {
		// Convert mono to stereo (duplicate each sample)
		decodedData = make([]int16, len(wavBuffer.Data)*2)
		for i, val := range wavBuffer.Data {
			sample := int16(val)
			decodedData[i*2] = sample   // Left
			decodedData[i*2+1] = sample // Right
		}
		numChannels = 2
	} else {
		decodedData = make([]int16, len(wavBuffer.Data))
		for i, val := range wavBuffer.Data {
			decodedData[i] = int16(val)
		}
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()

	mp3Encoder := mp3.NewEncoder(wavBuffer.Format.SampleRate, numChannels)
	if err := mp3Encoder.Write(out, decodedData); err != nil {
		return fmt.Errorf("failed to encode MP3: %w", err)
	}

	return nil
}
