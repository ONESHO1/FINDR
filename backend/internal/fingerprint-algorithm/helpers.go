package fingerprintalgorithm

import (
	"fmt"
	"math"
	"math/cmplx"

	"github.com/ONESHO1/FINDR/backend/internal/log"
)

const (
	cutOffFrequency 	= 5000.0 // 5kHz
	downSampleRatio		= 4
	frequencyBinSize 	= 1024
	hopSize 			= frequencyBinSize / 32

	targetZoneSize		= 5
)

type Peak struct {
	Time float64
	// Freq complex128
	FreqIdx int
}

func Spectrogram(sample []float64, sampleRate int) ([][]complex128, error) {
	sampleAfterFilter := LowPassFilter(cutOffFrequency, float64(sampleRate), sample)

	downedSample, err := Downsample(sampleAfterFilter, sampleRate, sampleRate / downSampleRatio)
	if err != nil {
		log.Logger.WithError(err).Error("Unable to Downsample.")
		return nil, err
	}

	window := make([]float64, frequencyBinSize)
	for i := range window {
		window[i] = 0.54 - 0.46 * math.Cos(2 * math.Pi * float64(i) / (float64(frequencyBinSize) - 1))
	}

	var spectrogram [][]complex128

	for start := 0; start + frequencyBinSize <= len(downedSample); start += hopSize {
		end := start + frequencyBinSize

		bin := make([]float64, frequencyBinSize)
		copy(bin, downedSample[start : end])

		for j := range window {
			bin[j] *= window[j]
		} 

		spectrogram = append(spectrogram, FastFourierTransform(bin))
	}

	return spectrogram, nil
}


/* 
ts uses a transfer function H(s) = 1 / (1 + sRC), 
where RC is the time constant to reduce the strength of high frequencies above the cut off frequency 
(often noise and not useful for recognition).

basically blurs the signal so that the high frequencies are smoothed out while not doing anything to the low frequencies.

Should've paid attention in signals and systems classes
*/
func LowPassFilter(cutOffFrequency, sampleRate float64, sample []float64) []float64 {
	rc := 1.0 / (2 * math.Pi * cutOffFrequency)
	dt := 1.0 / sampleRate
	alpha := dt / (rc + dt)

	var prev float64 = 0
	singalAfterFilter := make([]float64, len(sample))

	for i, input := range sample {
		if i == 0 {
			singalAfterFilter[i] = input * alpha
		} else {
			singalAfterFilter[i] = alpha * input + (1 - alpha) * prev
		}
		prev = singalAfterFilter[i]
	}
	return singalAfterFilter
}

/*
for every 4 samples of original, we only want 1 sample
so we just take the average of 4 blocks and replace it with the average

This reduces computation time in the following steps
*/
func Downsample(sampleAfterFilter []float64, sampleRate, target int) ([]float64, error) {
	if sampleRate <= 0 || target <= 0 {
		err := fmt.Errorf("sample rates must be above 0, sampleRate: %d | target: %d", sampleRate, target)
		return nil, err
	}

	if target > sampleRate {
		return nil, fmt.Errorf("target must be below current sample rate (we are downsampling smh), sampleRate: %d | target: %d", sampleRate, target)
	}

	ratio := sampleRate / target
	
	var resampled []float64
	for i := 0 ; i < len(sampleAfterFilter) ; i += ratio {
		end := i + ratio
		if end > len(sampleAfterFilter) {
			end = len(sampleAfterFilter)
		}

		sum := 0.0
		for j := i ; j < end ; j++ {
			sum += sampleAfterFilter[j]
		}
		avg := sum / float64(end - i)
		resampled = append(resampled, avg)
	}

	return resampled, nil
}

// Should've paid attention in complex variables class
func FastFourierTransform(input []float64) []complex128 {
	complexArray := make([]complex128, len(input))
	for i, v := range input {
		complexArray[i] = complex(v, 0)
	}

	fftRes := make([]complex128, len(complexArray))
	copy(fftRes, complexArray)
	return recursiveFFT(fftRes)
}

// implements FFt recursively
func recursiveFFT(input []complex128) []complex128 {
	n := len(input)
	if n <= 1 {
		return input
	}

	even := make([]complex128, n / 2)
	odd := make([]complex128, n / 2)	
	for i := 0 ; i < n / 2 ; i++ {
		even[i] = input[2 * i]
		odd[i] = input[2 * i + 1]
	}

	even = recursiveFFT(even)
	odd = recursiveFFT(odd)

	fftRes := make([]complex128, n)
	for k := 0 ; k < n / 2 ; k++ {
		t := complex(math.Cos(-2 * math.Pi * float64(k) / float64(n)), math.Sin(-2 * math.Pi * float64(k) / float64(n)))
		fftRes[k] = even[k] + t * odd[k]
		fftRes[k + n / 2] = even[k] - t * odd[k]
	}

	return fftRes
}

/*
Gets the peaks (brightest points) from the spectrogram.
It's often the stuff that identifies (is unique to) a particular song.
*/
func GetPeaksFromSpectrogram(spectrogram [][]complex128, sampleRate int) []Peak {
	if len(spectrogram) == 0 {
		return []Peak{}
	}

	type maxes struct {
		maxMagnitude 	float64
		maxFrequency 	complex128
		frequencyIndex 	int
	}

	// split into different logarithmic bands (just how sound works) to mimic how humans percieve sounds
	bands := []struct{ min, max int}{{0, 10}, {10, 20}, {20, 40}, {40, 80}, {80, 160}, {160, 512}}

	var peaks []Peak
	// get length (in seconds) for a single bin (slice)
	downsampledSampleRate := float64(sampleRate) / float64(downSampleRatio)
	binDuration := float64(hopSize) / downsampledSampleRate 		// hope ts works

	// iterate over slices
	for i, bin := range spectrogram {
		binMaxes := []maxes{}

		// go through each band
		for _, band := range bands {
			var maxi maxes
			var magMax float64
			
			// get max frequency for current band
			for j, freq := range bin[band.min : band.max] {
				magnitude := cmplx.Abs(freq) // intensity
				if magnitude > magMax {
					magMax = magnitude
					freqIdx := band.min + j
					maxi = maxes{magnitude, freq, freqIdx}
				}
			}
			// loudest/most intense from current band
			binMaxes = append(binMaxes, maxi)
		}

		// add the strongest peak from every frequency band
		// makes it  more resistant to background noise
		for _, peakInfo := range binMaxes {
			if peakInfo.maxMagnitude > 0 {
				peakTime := float64(i) * binDuration
				freqIdx := peakInfo.frequencyIndex
				peaks = append(peaks, Peak{Time: peakTime, FreqIdx: freqIdx})
			}
		}
	}

	return peaks
}

// Create actual hashes for pairs of peaks that are nearby
func Fingerprint(peaks []Peak, songID uint32) map[uint32]Couple {
	fingerprints := map[uint32]Couple{}

	// use each peak as an anchor point
	for i, anchor := range peaks {
		// change target zone size in the constants (ig the paper or the article said 5, thats what i have in my notes)
		for j := i + 1 ; j < len(peaks) && j <= i + targetZoneSize ; j++ {
			target := peaks[j]

			hash := hash(anchor, target)
			anchorTimeMs := uint32(anchor.Time * 1000) 	// must be in milliseconds for some reason

			fingerprints[hash] = Couple{anchorTimeMs, songID}
		}
	}

	return fingerprints
}

// create a hash for a anchor target pair
func hash(anchor, target Peak) uint32 {
	anchorFrequency := anchor.FreqIdx
	targetFrequency := target.FreqIdx
	// time difference in milliseconds also
	deltaMs := uint32((target.Time - anchor.Time) * 1000)

	/* 
	ripped the hashing straight out of the research paper 
	9 bits for anchor frequency
	9 bits for target frequency
	14 bits for time difference

	so a 32 bit hash
	*/
	address := uint32(anchorFrequency<<23) | uint32(targetFrequency<<14) | deltaMs

	return address
}