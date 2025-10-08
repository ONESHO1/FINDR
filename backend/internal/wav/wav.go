package wav

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ONESHO1/FINDR/backend/internal/log"
	"github.com/sirupsen/logrus"
)

// convert the audio file to a wav file
func ConvertToWav(filePath string, channels int) (wavFilePath string, err error) {
	_, err = os.Stat(filePath)
	if err != nil {
		log.Logger.WithError(err).WithField("input Path", filePath).Error("Input file for WAV conversion not found")
		return "", err
	}

	// we want single channel audio for the fingerprinting process
	if channels != 1 {
		channels = 1
	}

	fileExtention := filepath.Ext(filePath)

	wavFilePath = strings.TrimSuffix(filePath, fileExtention) + ".wav"

	// TODO: use a temporary file in case wav already exists

	cmd := exec.Command(
		"ffmpeg",
		"-y",					// overwrite output file if it already exists
		"-i", 					// set input 
		filePath,				// input file 
		"-c", 					// set Codec
		"pcm_s16le",			// Pulse Code Modulation (standard for raw, uncompressed audio) | signed 16-bit, little-endian. This is the standard bit depth and byte order for CD-quality audio.
		"-ar", 					// Audio Rate
		"44100",				// 44100 Hz (written in the research paper)
		"-ac", 					// set number of audio channels
		fmt.Sprint(channels),	// 1 chanel
		wavFilePath,			// output file path
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Logger.WithFields(logrus.Fields{
			"input path":    filePath,
			"output path":   wavFilePath,
			"ffmpeg_output": output,
			"error": err,
		}).Error("ffmpeg failed to convert to WAV")

		return "", err
	}

	return wavFilePath, nil
}

type WavInformation struct {
	Channels   int
	SampleRate int
	Data       []byte
	Duration   float64
}

// got ts from the internet, lets hope it works
type WavHeader struct {
	ChunkID       [4]byte
	ChunkSize     uint32
	Format        [4]byte
	Subchunk1ID   [4]byte
	Subchunk1Size uint32
	AudioFormat   uint16
	NumChannels   uint16
	SampleRate    uint32
	BytesPerSec   uint32
	BlockAlign    uint16
	BitsPerSample uint16
	Subchunk2ID   [4]byte
	Subchunk2Size uint32
}

// get the required wav informaton from the header of the wav file
func WavInfo(filePath string) (*WavInformation, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Logger.WithError(err).WithField("file Path", filePath).Error("Can't read wav file")
		return nil, err
	}


	// till 44 is the header 
	if len(data) < 44 {
		err := fmt.Errorf("file size is %d bytes, which is smaller than a 44-byte WAV header", len(data))
		log.Logger.WithError(err).WithField("file Path", filePath).Error(err)
		return nil, err
	}

	var header WavHeader
	err = binary.Read(bytes.NewReader(data[:44]), binary.LittleEndian, &header)
	if err != nil {
		log.Logger.WithError(err).WithField("file Path", filePath).Error("error reading the header")
		return nil, err
	}
	if string(header.Format[:]) != "WAVE" || string(header.ChunkID[:]) != "RIFF" || header.AudioFormat != 1 {
		err := fmt.Errorf("wrong WAV header format")
		log.Logger.WithFields(logrus.Fields{
			"filePath":    filePath,
			"chunkID":     string(header.ChunkID[:]),
			"format":      string(header.Format[:]),
			"audioFormat": header.AudioFormat,
		}).Error(err)
		return nil, err
	}

	info := &WavInformation{
		Channels: int(header.NumChannels),
		SampleRate: int(header.SampleRate),
		Data: data[44:],
	}

	// must be PCM formal during the conversion for this to be accurate
	if header.BitsPerSample == 16 {
		info.Duration = float64(len(info.Data)) / float64(int(header.NumChannels)*2*int(header.SampleRate))
	} else {
		err := fmt.Errorf("wrong bit depth: expected 16, got %d", header.BitsPerSample)
		log.Logger.WithField("filePath", filePath).Error(err)
		return nil, err
	}

	return info, nil
}

// decoding/normalization of the raw byte stream into a slice of float64 numbers, where each number represents the sound wave's amplitude at a specific point in time.
func Samples(input []byte) ([]float64, error) {
	if len(input) % 2 != 0 {
		err := fmt.Errorf("audio data has an odd number of bytes (%d), prolly corrupted", len(input))
		log.Logger.Error(err)
		return nil, err
	}

	size := len(input) / 2
	output := make([]float64, size)

	for i := 0 ; i < len(input) ; i += 2 {
		// each pair of bytes represents one sample.
		
		/*
		takes a pair of bytes and interprets them as a single 16-bit number. 
		It's then cast to int16, which can represent both positive and 
		negative values (from -32,768 to 32,767), just like a sound wave goes above and 
		below a central zero point.
		*/
		sample := int16(binary.LittleEndian.Uint16(input[i: i + 2]))

		// scale samples to range [-1, 1]
		output[i / 2] = float64(sample) / 32768.0
	}

	return output, nil
}