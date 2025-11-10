# FINDR

Shazam was the first app that blew my mind. Did you know they launched in 2002? (Crazy right). Anyways, I was always fascinated by Shazam's "magic." Until I came across a video that explained a brief overview of the audio fingerprinting process, I thought, "I could build that." And then I realized why Shazam is worth $400 million.

FINDR is a working implementation of an audio fingerprinting and recognition algorithm. It can analyze a large library of music to create a database of "fingerprints" and then identify a song from a short audio recording by matching its fingerprint against that database.

This project is a deep dive into the core concepts of audio fingerprinting, database optimization, concurrent programming, and me learning Go, based on the principles outlined in the original Shazam research paper.

## Features

- **Song Ingestion:** Add song(s) to the database using a Spotify playlist link.
    
- **Audio Sourcing:** Automatically finds and downloads song audio from YouTube.
    
- **Live Recording:** Identifies music in real-time by recording audio from your microphone.
    
- **Robust Matching:** The algorithm is designed to be noise-resistant, allowing it to match songs even with significant background noise.
    
- **Scalable Database:** Uses PostgreSQL to efficiently store and query millions of song fingerprints.
    

## Technologies Used

- **Core:** [Go (Golang)](https://go.dev/)
    
- **Database:** [PostgreSQL](https://www.postgresql.org/) (using the `pgx` driver)
    
- **Audio Processing:** [FFmpeg](https://ffmpeg.org/) (for audio conversion), [malgo](https://github.com/gen2brain/malgo) (for audio recording)
    
- **APIs & Sourcing:** [Spotify Web API](https://developer.spotify.com/documentation/web-api), [yt-dlp](https://github.com/yt-dlp/yt-dlp) (for YouTube downloads)
    

---

## Getting Started

### Prerequisites

You must have the following tools installed and available in your system's `PATH`:

- [Go](https://go.dev/doc/install) (version 1.20 or later)
    
- [PostgreSQL](https://www.postgresql.org/download/) (a running instance)
    
- [FFmpeg](https://ffmpeg.org/download.html)
    
- [yt-dlp](https://www.google.com/search?q=https://github.com/yt-dlp/yt-dlp%23installation)
    

### Installation & Setup

1. **Clone the repository:**
    
    
    
    ``` 
    git clone https://github.com/your-username/findr.git
    cd findr/backend
    ```
    
2. **Install Go dependencies:**
    
    
    ```
    go mod tidy
    ```
    
3. **Configure Environment Variables:** Create a `.env` file in the `backend/` directory. You will need to add your credentials:
    
    
    ```
    # PostgreSQL Credentials
    POSTGRES_USERNAME=your_username
    POSTGRES_PASSWORD=your_password
    POSTGRES_DATABASE_NAME=findr_db
    
    # Spotify API Credentials
    SPOTIFY_CLIENT_ID=your_client_id
    SPOTIFY_CLIENT_SECRET=your_client_secret
    ```
    

### Usage

The application is run from the command line inside the `backend/cmd` directory.

#### 1. Add Songs to the Database

Run the `add` command with a link to a Spotify playlist. This will download each song, process it, and add its fingerprints to the database.


```Bash
go run ./main.go add "httpsT://open.spotify.com/playlist/..."
```

#### 2. Identify a Song

Run the `findr` command. This will record audio from your default microphone, process it, and print the best match from your database.


```Bash
go run ./main.go findr
```

---

## How It Works: A Deep Dive

The system is split into two main pipelines: **Ingestion** and **Matching**. The system's accuracy relies on both pipelines being 100% algorithmically identical.

<img width="10800" height="2720" alt="Basic working of the fingerprinting algorithm - excalidraw" src="https://github.com/user-attachments/assets/3936173f-ed53-41bc-a0aa-38712735c817" />

### 1. Ingestion Pipeline (Adding a Song)

This is the process of converting a full-length audio file into a set of searchable fingerprints.

1. **Download & Standardize (`downloader.go`, `wav.go`):**
    
    - A song (e.g., "Instant Crush") is downloaded from YouTube.
        
    - `ffmpeg` converts it to a **1-channel (mono), 16-bit, 44.1kHz WAV file**. This standardization is the most critical step for ensuring all audio is processed identically.
        
    - The raw WAV bytes are read and converted into a `[]float64` slice (a normalized audio "sample").
        
2. **Spectrogram (`helpers.go`):**
    
    - The sample is passed through a `LowPassFilter` (removing >5000Hz noise) and `Downsample`'d (to 11,025Hz) to speed up analysis.
        
    - The sample is processed using a **Short-Time Fourier Transform (STFT)**. This converts the 1D audio wave into a 2D **spectrogram** (Time vs. Frequency), showing which "notes" are "loud" at each moment.
        
3. **Peak Finding (`helpers.go`):**
    
    - The spectrogram is analyzed, and the loudest point (the "peak") is found for several logarithmic frequency bands (e.g., 0-10Hz, 10-20Hz, etc.) for each slice of time.
        
    - This **"robust" method** (taking all band peaks, not using a strict average) is what makes the algorithm resistant to background noise.
        
    - The result is a list of `Peak` structs, each with a `Time` (in seconds) and a `FreqIdx` (a number representing its "note").
        
4. **Hashing (`helpers.go`):**
    
    - The list of peaks is iterated. Each peak is an "anchor."
        
    - For each "anchor," the algorithm looks at the next 5 "target" peaks (`targetZoneSize`).
        
    - A `hash` is created for each `(anchor, target)` pair. This hash is a single 32-bit number that encodes three facts: `anchor.FreqIdx`, `target.FreqIdx`, and the `time_delta` between them.
        
    - This "constellation" hash is the final fingerprint.
        
5. **Storage (`postgres.go`):**
    
    - The song is added to the `songs` table, and a new `song_id` is generated.
        
    - All generated hashes are stored in the `fingerprints` table, along with their `song_id` and the absolute `anchor_time_ms` of the peak that created them.
        

### 2. Matching Pipeline (Finding a Song)

This process runs the **exact same algorithm** on a recorded clip.

1. **Record & Standardize (`match.go`):**
    
    - `recordFromMic` captures audio in **1-channel (mono)** format, matching the database standard.
        
    - `find` calls `wav.ConvertToWav(filePath, 1)`. This is a robust step: it ensures the file is mono, even if the input was a stereo file.
        
    - The standardized audio is converted to a `[]float64` sample.
        
2. **Fingerprint Query (`findmatches.go`):**
    
    - The _identical_ `Spectrogram` -> `GetPeaksFromSpectrogram` -> `Fingerprint` pipeline is run on the sample.
        
    - This generates a `sampleFingerprintMap` (a map of `hash -> sample_anchor_time_ms`).
        
3. **Database Lookup (`postgres.go`):**
    
    - The list of all hashes from the recording is sent to the database.
        
    - The `idx_fingerprints_hash` index is used to instantly fetch all rows from the `fingerprints` table that match any of the query hashes.
        
4. **Scoring (The "Magic" in `findmatches.go`):**
    
    - The database returns a list of matches from all songs. The code first groups these matches by `song_id`.
        
    - For each potential song, it compares its list of time-pairs (`{sample_time, db_time}`).
        
    - It uses a nested `O(N^2)` loop to check if the _relative time difference_ between two peaks in the sample is the same as the _relative time difference_ between the same two peaks in the database.
        
    - **Example:**
        
        - `sample_time[i] - sample_time[j]` = 2.5 seconds
            
        - `db_time[i] - db_time[j]` = 2.5 seconds
            
        - If they match (within a `TOLERANCE`), it's a "win." The `count` is incremented.
            
    - The song with the highest `count` is the final prediction.
        

---

## Known Issues & Limitations

- **Critical: Memory Usage:** The `Spectrogram` function (`helpers.go`) loads the _entire_ song's spectrogram into RAM, which can be **1-2 GB per song**. This causes `fatal error: cannot allocate memory` when ingesting multiple songs in parallel.
    
- **Critical: Concurrency:** The `download` function (`downloader.go`) launches an **unlimited** number of goroutines (one for every song in a playlist). This will crash the system on large playlists due to the memory issue above.
    
- **Critical: Scoring Performance:** The scoring logic in `findmatches.go` is **`O(N^2)`** (quadratic). This makes the `findr` command very slow as the number of matches increases.

- As a result of these and my inability to pay attention, the whole process is pretty slow compared to shazam, but it already took me 2 months to implement it, so I'm going to take a break before returning to fix these.
    

## Roadmap

This is the planned list of improvements for the project.

- [ ] **Refactor: O(N) Scoring:** Replace the `O(N^2)` scoring loop in `findmatches.go` with a faster `O(N)` histogram-based approach.
    
- [ ] **Refactor: Memory Optimization:** Combine `Spectrogram` and `GetPeaksFromSpectrogram` into a single, streaming function that doesn't store the full spectrogram in memory.
    
- [ ] **Refactor: Concurrency:** Get it to start fingerprinting the audio while the snippet is being recorded
    
- [ ] **Feature: Better Error Handling:** Improve `yt-dlp` error handling for age-restricted videos, private videos, and unavailable formats.
    
- [ ] **Infra: Dockerize:** Containerize the application and its PostgreSQL database using `docker-compose`.
    
- [ ] **Refactor: Dependency Injection:** Use dependency injection for the database client (`DbClient`).
    
- [ ] **Feature: Frontend:** Build a frontend web application. The recording and fingerprinting should happen on the client-side (in-browser) to reduce server load.
    
- [ ] **Infra: Deployment:** Deploy the finalized application to a cloud provider (e.g., AWS).
    
- [ ] **Infra: Microservices:** Refactor the system into a microservice architecture (e.g., using Kafka for a job queue).

---

## License

This project is licensed under the **MIT License**.

## Sources & Acknowledgments

I Could not have got it working if not for these resources.

- [An Industrial-Strength Audio Search Algorithm (Shazam Paper) - Wang, 2003](https://www.ee.columbia.edu/~dpwe/papers/Wang03-shazam.pdf)
    
- [How Does Shazam Work? - Medium](https://medium.com/@anaharris/how-does-shazam-work-d38f74e41359)
    
- [Shazam-It: Music Processing, Fingerprinting, and Recognition - Toptal](https://www.toptal.com/algorithms/shazam-it-music-processing-fingerprinting-and-recognition)
    
- [ECE 472 Project: Audio Fingerprinting - Rochester](https://hajim.rochester.edu/ece/sites/zduan/teaching/ece472/projects/2019/AudioFingerprinting.pdf)
    
- [Decoding Shazam: How Does Music Recognition Work? - TechAhead](https://www.techaheadcorp.com/blog/decoding-shazam-how-does-music-recognition-work-with-shazam-app/)
    
- [Audio Fingerprinting with Python and NumPy](https://drive.google.com/file/d/1ahyCTXBAZiuni6RTzHzLoOwwfTRFaU-C/view)


