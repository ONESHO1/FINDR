package match

import (
	"bytes"
	"errors"
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
const RECORDING_TIME = 25 // in seconds

func RecordAndFind() {
	path, err := recordFromMic()
	if err != nil {
		log.Logger.WithError(err).Error("Failed to record audio")
		return
	}

	err = find(path)
	if err != nil {
		log.Logger.WithError(err).Error("Could not Find match")
		return
	}
}

/*
Gang ima be real w you, there might be an issue with the audio recording,
the audio goes silent at some points,
i dont know if it will be clear enough for the fingerprinting to be accurate, lets see

I'm a dumbass, record with the audio source pointing at the microphone
*/
func recordFromMic() (string, error) {
	// use windows' audio thing
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

	// just testing
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = 1
	deviceConfig.SampleRate = 44100

	// Use a channel to safely pass data from the audio thread to the main thread.
	dataChan := make(chan []byte, 100) // Buffered channel (max 100 chunks)

	sizeInBytes := uint32(malgo.SampleSizeInBytes(deviceConfig.Capture.Format))

	// audio call back function which is called repeatedly on a seperate thread (according to their docs) | copy the data as it pSample will be used by the function imeediately
	onRecvFrames := func(_, pSample []byte, frameCount uint32) {
		sampleCount := frameCount * deviceConfig.Capture.Channels * sizeInBytes

		// copy data
		copiedData := make([]byte, sampleCount)
		copy(copiedData, pSample)

		// Send the data to the channel.
		dataChan <- copiedData
	}

	deviceCallbacks := malgo.DeviceCallbacks{
		Data: onRecvFrames,
	}

	// system's default mic
	device, err := malgo.InitDevice(ctx.Context, deviceConfig, deviceCallbacks)
	if err != nil {
		log.Logger.WithError(err).Fatal("Failed to initialize audio device")
		return "", err
	}
	defer device.Uninit()

	/*
		Use a WaitGroup and a separate goroutine to collect data from the channel.
		This ensures all 'append' operations happen in one safe place.
	*/
	var capturedData bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	// use seperate collectors
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
		// close channel for failures
		close(dataChan)
		wg.Wait()
		return "", err
	}

	time.Sleep(RECORDING_TIME * time.Second)

	_ = device.Stop()
	log.Logger.Info("Recording stopped, saving to file...")

	// Close channel
	close(dataChan)
	// Wait for collection goroutines
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

	// hah, gottem
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

func find(filePath string) error {
	// just doing this incase I want to accept audio files in the future (since I'm already recording in single channel)
	// monoFilePath, err := wav.ConvertToWav(filePath, 1)
	// if err != nil {
	// 	log.Logger.WithError(err).Error("Failed to convert query audio to mono")
	// 	return err
	// }
	// defer os.Remove(monoFilePath)

	wavInfo, err := wav.WavInfo(filePath)
	if err != nil {
		log.Logger.WithError(err).Error("error reafing wav file info")
		return err
	}

	samples, err := wav.Samples(wavInfo.Data)
	if err != nil {
		log.Logger.WithError(err).Error("error generating samples")
		return err
	}

	matches, duration, err := FindMatches(samples, wavInfo.Duration, wavInfo.SampleRate)
	if err != nil {
		log.Logger.WithError(err).Error("error finding samples")
		return err
	}

	if len(matches) == 0 {
		log.Logger.Error("No Matches")
		return errors.New("NO MATCHES FOUND")
	}

	topMatches := matches
	if len(matches) > 10 {
		topMatches = matches[:10]
	}
	fmt.Println("Top Matches ->")
	for _, match := range topMatches {
		fmt.Printf("\t- %s by %s, score: %.2f\n", match.SongTitle, match.SongArtist, match.Score)
	}

	fmt.Printf("\nSearch took: %s\n", duration)
	res := topMatches[0]
	fmt.Printf("\nFinal prediction: %s by %s , score: %.2f\n", res.SongTitle, res.SongArtist, res.Score)

	return nil
}
