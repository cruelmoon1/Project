Run the application:

Bash
go run cmd/main.go
The server will start on http://localhost:8080.

Option 2: Running with Docker Compose
Ensure Docker is running.

Build and start the containers:

Bash
docker compose up --build
The API will be available at http://localhost:8080.

Монгол Хувилбар
Шаардлагатай зүйлс
Go (хувилбар 1.18 болон түүнээс дээш)

Docker болон Docker Compose

Postman (API турших зорилгоор)

Ажиллуулах заавар
Сонголт 1: Команд мөр (CMD) ашиглан локал дээр ажиллуулах
Төслийн үндсэн хавтас руу орно.

Шаардлагатай сангуудыг бэлдэх:

Bash
go mod tidy
Програмыг ажиллуулах:

Bash
go run cmd/main.go
Сервер http://localhost:8080 хаяг дээр ажиллах болно.

Сонголт 2: Docker Compose ашиглан ажиллуулах
Docker програм ажиллаж байгаа эсэхийг шалгана.

Контейнерийг бүтээн ажиллуулах:

Bash
docker compose up --build
API нь http://localhost:8080 хаяг дээр бэлэн болно.