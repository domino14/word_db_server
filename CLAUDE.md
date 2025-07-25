# Word DB Server

Word DB Server is a Go backend server that powers multiple frontend applications for word study in scrabble-like games.

The most specialized of these applications is WordVault, which
allows users to study alphagrams and track their progress using
spaced repetition algorithms

## Core Data Models

**Alphagram:**

- Alphabetically sorted letters that can form one or more words
- Each word must use exactly the letters provided (including duplicates)
- Example: "NRU" creates words "RUN" and "URN"
- Belongs to a specific lexicon (dictionary/language ruleset)
- Same alphagram may have different valid words across lexicons

**Word:**

- Single valid word that can be formed from an alphagram
- Contains lexicon symbols indicating word status (new, collins-only, etc.)
- Lexicon symbols show word metadata, not lexicon membership

**Card (WordVault-specific):**

- Base unit for spaced repetition study
- User and lexicon specific (same alphagram = different cards per user)
- Tracks study progress and schedules reviews based on performance

**Deck (WordVault-specific):**

- Optional grouping mechanism for cards
- Allows different study strategies and scheduling parameters
- Example: separate deck for longer words with different retention targets

## Technical Architecture

### Server Framework

- **Framework:** Built using Connect RPC. Types defined in \*.proto files
- **HTTP Server:** Configurable port with graceful shutdown handling
- **Middleware:** Uses alice middleware for logging and request handling
- **Authentication:** JWT-based authentication with Connect interceptors

### Database Layer

- **Primary Database:** PostgreSQL with pgx/v5 driver and connection pooling
  - Stores user data including wordvault_cards, wordvault_params, wordvault_decks
  - Uses JSONB fields for FSRS (spaced repetition) algorithm data
- **Migration System:** golang-migrate/v4 with migrations in `db/migrations/`
- **Query Generation:** sqlc for type-safe SQL queries from `db/queries/`
- **Lexicon Data:** SQLite databases for word/alphagram lookup
  - Stored in `DataPath/lexica/db/` directory
  - Contains linguistic data optimized for fast searches

### API Structure

**Connect RPC Services:**

- **WordVaultService** (authenticated): Card management, spaced repetition scheduling, deck operations
- **QuestionSearcher:** Alphagram and word searching functionality
- **Anagrammer:** Anagram generation, blank tile challenges, build challenges
- **WordSearcher:** Simple word lookups and definitions

### Key Internal Components

- **internal/wordvault:** FSRS spaced repetition logic using go-fsrs/v3 library
- **internal/searchserver:** SQLite-based word and alphagram search engine
- **internal/anagramserver:** GADDAG/KWG-based anagram generation using word-golib
- **internal/auth:** JWT authentication and context management
- **dbmaker:** Utility for creating and updating SQLite lexicon databases

### Deployment

- **Containerization:** Docker-based deployment
- **CI/CD:** GitHub Actions pipeline for automated testing and deployment
- **Database Management:** Migrations run automatically on startup

## Development Rules

- **Do not run migrations or start the dev server directly**
- The development environment runs via docker-compose in a separate directory
- Database and server are containerized and managed externally
