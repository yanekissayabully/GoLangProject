# DriveBai

**Car marketplace & rental — backend-first in Go**

DriveBai is a backend-first platform that connects car owners with drivers. It provides a clean REST API and WebSocket-based real-time chat, enabling users to browse listings, communicate, and manage rental requests seamlessly. The project is built as a monolith with a modular internal structure, ready to be consumed by any mobile or web client.

---

## 🚀 Features

### 🔐 Authentication & RBAC
- JWT access and refresh tokens  
- Email OTP verification  
- Role-based access control: **Driver**, **Owner**, **Admin**

### 👤 User Onboarding
- Profile photo upload  
- Document upload for verification  
- Onboarding status tracking

### 🚗 Car Listings
- Full CRUD operations for car listings  
- Photo and document attachments  
- Location and availability management

### ❤️ Favorites
- Like/unlike listings  
- Personal liked list for quick browsing

### 💬 Chat + Requests
- REST endpoints + WebSocket live events  
- Structured rental requests (accept/decline/cancel)  
- Message attachments

### 📄 Documentation & Ops
- OpenAPI + Swagger UI  
- Docker Compose for local development  
- SQL migrations

---

## 🧱 Technology Stack

| Layer          | Choice             | Why it fits the course |
|----------------|--------------------|------------------------|
| Language       | Go 1.22            | Fast, simple concurrency; idiomatic REST services |
| Routing        | chi                | Lightweight router + middleware (auth, logging, CORS) |
| Database       | PostgreSQL + pgx   | Relational schema with FK + indexes; strong SQL practice |
| Migrations     | golang-migrate     | Repeatable schema changes; works in Docker CI/dev |
| Auth           | JWT + refresh tokens | Secure login + role-based access |
| Realtime       | WebSocket hub      | Chat events + request updates; demonstrates goroutines |
| Tooling        | OpenAPI + Docker Compose | Swagger UI demo + one-command local setup |

---

## 🗂️ Project Structure (Monolith)

```
.
├── cmd/                # Application entry points
├── internal/           # Core packages (auth, cars, chat, etc.)
├── migrations/         # SQL migrations
├── docs/               # OpenAPI/Swagger specs
├── docker-compose.yml  # Local dev environment
└── README.md
```

---

## 🧩 Key Entities & Relationships

- **User** → Cars (owner)  
- **User** ↔ Cars (likes)  
- **Car** → Chats (driver–owner per car)  
- **Chat** → Messages, Requests, Attachments

---

## 📡 API Highlights

### Auth
`/api/v1/auth/register` · `/login` · `/verify-email` · `/refresh-token` · `/forgot-password` · `/reset-password`

### Cars & Listings
`/api/v1/listings` (public browse)  
`/api/v1/cars` (CRUD + photos/docs)  
`/api/v1/me/likes` (favorites)

### Chats (REST + WebSocket)
`/api/v1/ws?token=` — WebSocket connection  
`/api/v1/chats` · `/messages` · `/requests` · `/attachments`

---

## 👥 Team & Roles

| Role               | Responsibilities |
|--------------------|------------------|
| **Team Lead**      | Architecture decisions, PR reviews, integration & demo |
| **Core Backend Dev** | Implement APIs + DB layer (cars, chats, auth) |
| **QA Engineer**    | Test plan, Postman collections, regression + edge cases |
| **Scrum Master**   | Sprint planning, backlog hygiene, standups & reports |

---


## ⚠️ Risks & Mitigations

| Risk                          | Mitigation |
|-------------------------------|------------|
| WebSocket edge cases          | Fallback polling + integration tests |
| Migration conflicts           | One owner per migration + PR review |
| Email deliverability          | Dev console fallback + domain auth in production |

---

## 🛠️ Getting Started

### Prerequisites
- Go 1.22+
- Docker & Docker Compose
- PostgreSQL (or use Docker)

### Run locally
```bash
# Clone the repository
git clone https://github.com/your-org/drivebai.git
cd drivebai

# Start database and services
docker-compose up -d

# Run migrations
make migrate-up

# Start the server
go run cmd/main.go
```

Swagger UI will be available at `http://localhost:8080/swagger`
