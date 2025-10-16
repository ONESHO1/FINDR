package db

import (
	"database/sql"
	"fmt"

	"github.com/ONESHO1/FINDR/backend/internal/utils"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	
	"github.com/ONESHO1/FINDR/backend/internal/fingerprint-algorithm"
)

type PostgresClient struct {
	db *sql.DB
}

// serves up a new client of type PostgresClient
func newPostgresClient(connectionString string) (*PostgresClient, error) {
	db, err := sql.Open("pgx", connectionString)
	if err != nil {
		return nil, fmt.Errorf("error connecting to PostgreSQL: %w", err)
	}

	if err = db.Ping() ; err != nil {
		return nil, fmt.Errorf("error pinging PostgreSQL: %w", err)
	}

	if err = createTables(db); err != nil {
		return nil, fmt.Errorf("error creating tables: %w", err)
	}

	return &PostgresClient{db: db}, nil
}

// creates the two tables need for FINDR, also creates an index to imporve fingerprint matching performance
func createTables(db *sql.DB) (error) {
	createSongsTable := `
	CREATE TABLE IF NOT EXISTS songs (
		id BIGSERIAL PRIMARY KEY,
		title TEXT NOT NULL,
		artist TEXT NOT NULL,
		key TEXT NOT NULL UNIQUE
	);
	`

	createFingerprintsTable := `
	CREATE TABLE IF NOT EXISTS fingerprints (
		address BIGINT NOT NULL,
		anchorTime INTEGER NOT NULL,
		songID BIGINT NOT NULL,
		PRIMARY KEY (address, anchorTime, songID)
	)
	`

	// dont even have to use it anywhere, postgres' query planner gives automatic efficiency gain (read on some website)
	createFingerprintsIndex := `
	CREATE INDEX IF NOT EXISTS 
	idx_fingerprints ON fingerprints (address);
	`

	_, err := db.Exec(createSongsTable)
	if err != nil {
		return fmt.Errorf("error creating songs table : %w", err)
	}

	_, err = db.Exec(createFingerprintsTable)
	if err != nil {
		return fmt.Errorf("error creating fingerprints table : %w", err)
	}


	_, err = db.Exec(createFingerprintsIndex)
	if err != nil {
		return fmt.Errorf("error creating fingerprints index: %w", err)
	}

	return nil
}

// closes the db connection
func (c *PostgresClient) Close() (error) {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// store fingerprints using a transaction (does nothing for duplicates)
func (c *PostgresClient) StoreFingerprints(fingerprints map[uint32]fingerprintalgorithm.Couple) (error) {
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO fingerprints (address, anchorTime, songID)
		VALUES ($1, $2, $3)
		ON CONFLICT (address, anchorTime, songID) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("error preparing statement: %w", err)
	}
	defer stmt.Close()

	for hash, couple := range fingerprints {
		if _, err := stmt.Exec(hash, couple.AnchorTimeMs, couple.SongID); err != nil {
			return fmt.Errorf("error executing statement: %w", err)
		}
	} 
	
	return tx.Commit()
}

// retrieve couples that match the addresses (hashes) | returns map where key is a hash and the value is a slice of all Couples found for that hash in the database.
func (c *PostgresClient) GetCouples(addresses []uint32) (map[uint32][]fingerprintalgorithm.Couple, error) {
	couples := make(map[uint32][]fingerprintalgorithm.Couple)

	addrsInt64 := make([]int64, len(addresses))
	for i, v := range addresses {
		addrsInt64[i] = int64(v)
	}

	query := "SELECT address, anchorTime, songID FROM fingerprints WHERE address = ANY($1)"
	rows, err := c.db.Query(query, addrsInt64)
	if err != nil {
		return nil, fmt.Errorf("error querying database : %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var address uint32
		var couple fingerprintalgorithm.Couple
		if err := rows.Scan(&address, &couple.AnchorTimeMs, &couple.SongID); err != nil {
			return nil, fmt.Errorf("error in scaning row : %w", err)
		}
		couples[address] = append(couples[address], couple)
	}

	return couples, nil
}

// return total number of songs
func (c *PostgresClient) TotalSongs() (int, error) {
	var count int
	err := c.db.QueryRow("SELECT COUNT(*) FROM songs").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error getting count : %w", err)
	}

	return count, nil
}

// register song and return the generated songID
func (c *PostgresClient) RegisterSong(songTitle, songArtist string) (uint32, error){
	songKey := utils.GenerateSongKey(songTitle, songArtist)

	var songID uint32

	query := "INSERT INTO songs (title, artist, key) VALUES ($1, $2, $3) RETURNING id"
	err := c.db.QueryRow(query, songTitle, songArtist, songKey).Scan(&songID)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			return 0, fmt.Errorf("song with key already exists: %w", err)
		}
		return 0, fmt.Errorf("failed to register song: %w", err)
	}

	return songID, nil
}

// A flexible, internal function to retrieve a single song by either its id or its unique key. | interface is the same as any (to make it flexiblt)
func (c *PostgresClient) GetSong(filterKey string, value interface{}) (Song, bool, error) {
	validFilterKeys := map[string]bool{"id": true, "key": true}

	if !validFilterKeys[filterKey] {
		return Song{}, false, fmt.Errorf("not a valid filter")
	}

	query := fmt.Sprintf("SELECT id, title, artist FROM songs WHERE %s = $1", filterKey)

	row := c.db.QueryRow(query, value)

	var song Song
	// TODO: Placing the database ID in the song's yt ID section, CHANGE TS
	err := row.Scan(&song.YouTubeID, &song.Title, &song.Artist)
	if err != nil {
		if err == sql.ErrNoRows {
			return Song{}, false, nil
		}
		return Song{}, false, fmt.Errorf("failed to retrieve song: %w", err)
	}

	return song, true, nil
}
 
// read the function name
func (c *PostgresClient) GetSongByID(songID uint32) (Song, bool, error) {
	return c.GetSong("id", songID)
}

// read the function name
func (db *PostgresClient) GetSongByKey(key string) (Song, bool, error) {
	return db.GetSong("key", key)
}

// delete a song by ID
func (db *PostgresClient) DeleteSongByID(songID uint32) error {
	_, err := db.db.Exec("DELETE FROM songs WHERE id = $1", songID)
	if err != nil {
		return fmt.Errorf("failed to delete song: %v", err)
	}
	return nil
}

// delete a table from the database
func (db *PostgresClient) DeleteCollection(collectionName string) error {
	_, err := db.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", collectionName))
	if err != nil {
		return fmt.Errorf("error deleting collection: %v", err)
	}
	return nil
}