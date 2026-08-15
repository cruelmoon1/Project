## Overview
This project provides a ledger service API. It includes account management and transaction processing capabilities.

## Getting Started

### Prerequisites
* Go (version 1.18 or higher)
* Docker and Docker Compose
* Postman (for API testing)

### Running Locally (CMD)
1. Navigate to the project root directory.
2. Initialize and download dependencies:
   ```bash
   go mod tidy
Run the application:

Bash
go run cmd/api/main.go
The server will start at http://localhost:8080.

Running Tests
To run all unit and integration tests locally, use:

Bash
go test ./...
Running with Docker
Ensure Docker is running.

Build and start the containers:

Bash
docker compose up --build
The API will be available at http://localhost:8080.

Project Reports
Postman Collection & API Testing: Tested core endpoints (POST /accounts, POST /transactions).

Input Validation & Error Handling: Verified error cases and constraint violations.

Repository Cleanup: Configured .gitignore for security and clean tracking.

Documentation: Integrated comprehensive setup and testing guides.