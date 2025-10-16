package match

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ONESHO1/FINDR/backend/internal/log"
	"github.com/ONESHO1/FINDR/backend/internal/wav"
	"github.com/gen2brain/malgo"
)

const RECORDINGS_DIR = "recordings"
const RECORDING_TIME = 20 // in seconds

/*
Gang ima be real w you, there might be an issue with the audio recording, 
the audio goes silent at some points, 
i dont know if it will be clear enough for the fingerprinting to be accurate, lets see
*/
func Record() (string, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(message string) {
		log.Logger.Debugf("malgo: %s", message)
	})
	if err != nil {
		log.Logger.WithError(err).Fatal("Failed to initialize audio context")
		return "", err
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	// ✨ FIX: Use Capture mode for a recording-only device.
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = 1
	deviceConfig.SampleRate = 44100

	// ✨ FIX: Use a channel to safely pass data from the audio thread to the main thread.
	dataChan := make(chan []byte, 100) // Buffered channel to avoid blocking the audio thread.
	
	sizeInBytes := uint32(malgo.SampleSizeInBytes(deviceConfig.Capture.Format))

	onRecvFrames := func(_, pSample []byte, frameCount uint32) {
		sampleCount := frameCount * deviceConfig.Capture.Channels * sizeInBytes

		// It's crucial to make a copy of the data before sending it to the channel.
		// The original pSample buffer will be reused by malgo.
		copiedData := make([]byte, sampleCount)
		copy(copiedData, pSample)

		// Send the copied data to the channel.
		dataChan <- copiedData
	}

	deviceCallbacks := malgo.DeviceCallbacks{
		Data: onRecvFrames,
	}
	device, err := malgo.InitDevice(ctx.Context, deviceConfig, deviceCallbacks)
	if err != nil {
		log.Logger.WithError(err).Fatal("Failed to initialize audio device")
		return "", err
	}
	defer device.Uninit()

	// ✨ FIX: Use a WaitGroup and a separate goroutine to collect data from the channel.
	// This ensures all 'append' operations happen in one safe place.
	var capturedData bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for data := range dataChan {
			capturedData.Write(data)
		}
	}()

	log.Logger.Infof("Recording for %d seconds...", RECORDING_TIME)
	err = device.Start()
	if err != nil {
		log.Logger.WithError(err).Fatal("Failed to start audio device")
		// Need to handle closing the channel if we fail to start
		close(dataChan)
		wg.Wait()
		return "", err
	}

	time.Sleep(RECORDING_TIME * time.Second)

	_ = device.Stop()
	log.Logger.Info("Recording stopped, saving to file...")
	
	// Close the channel to signal the collection goroutine to finish.
	close(dataChan)
	// Wait for the collection goroutine to process all remaining data.
	wg.Wait()


	if err := os.MkdirAll(RECORDINGS_DIR, os.ModePerm); err != nil {
		log.Logger.WithError(err).Error("Failed to create recordings directory")
		return "", fmt.Errorf("failed to create recordings dir: %w", err)
	}

	filename := fmt.Sprintf("rec_%d_%d.wav", time.Now().Unix(), rand.Intn(10000))
	outputPath := filepath.Join(RECORDINGS_DIR, filename)

	outputFile, err := os.Create(outputPath)
	if err != nil {
		log.Logger.WithError(err).WithField("outputPath", outputPath).Error("Failed to create output WAV file")
		return "", err
	}
	defer outputFile.Close()
	
	finalAudioData := capturedData.Bytes()
	dataSize := uint32(len(finalAudioData))
	sampleRate := uint16(deviceConfig.SampleRate)
	bitsPerSample := uint16(16)
	channels := uint16(deviceConfig.Capture.Channels)

	if err := wav.WriteWavHeader(outputFile, dataSize, sampleRate, bitsPerSample, channels); err != nil {
		log.Logger.WithError(err).Error("Failed to write WAV header")
		return "", err
	}

	if _, err := outputFile.Write(finalAudioData); err != nil {
		log.Logger.WithError(err).Error("Failed to write WAV data")
		return "", err
	}

	log.Logger.WithField("path", outputPath).Info("Recording saved successfully")
	return outputPath, nil
}