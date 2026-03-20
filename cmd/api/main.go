package main

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/drivebai/backend/internal/auth"
	"github.com/drivebai/backend/internal/config"
	"github.com/drivebai/backend/internal/database"
	"github.com/drivebai/backend/internal/email"
	"github.com/drivebai/backend/internal/handlers"
	"github.com/drivebai/backend/internal/middleware"
	"github.com/drivebai/backend/internal/repository"
	stripeService "github.com/drivebai/backend/internal/stripe"
	"github.com/drivebai/backend/internal/ws"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

//go:embed static/*
var staticFiles embed.FS

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if cfg.IsDevelopment() {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}

	// Connect to database
	ctx := context.Background()
	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("connected to database")

	// Initialize services
	jwtSvc := auth.NewJWTService(cfg.JWTSecret, cfg.JWTAccessTokenTTL, cfg.JWTRefreshTokenTTL)
	emailSvc := email.NewSender(cfg.SendGridAPIKey, cfg.SendGridFromEmail, cfg.SendGridFromName, cfg.AppDeeplinkScheme, cfg.AppBaseURL, logger)
	otpEmailSvc := email.NewOTPSender(cfg.MailerSendAPIKey, cfg.MailerFromEmail, cfg.MailerFromName, logger)

	// Initialize repositories
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewTokenRepository(db)
	loginOTPRepo := repository.NewLoginOTPRepository(db)
	docRepo := repository.NewDocumentRepository(db)
	carRepo := repository.NewCarRepository(db)
	carPhotoRepo := repository.NewCarPhotoRepository(db)
	carDocRepo := repository.NewCarDocumentRepository(db)
	likesRepo := repository.NewLikesRepository(db)
	chatRepo := repository.NewChatRepository(db)
	leaseRepo := repository.NewLeaseRequestRepository(db)

	// Ensure uploads directory exists
	uploadDir := cfg.UploadDir
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		logger.Error("failed to create uploads directory", "error", err)
		os.Exit(1)
	}

	// Initialize WebSocket hub
	wsHub := ws.NewHub(logger)
	go wsHub.Run()

	// Initialize Stripe service
	stripeSvc := stripeService.NewService(cfg.StripeSecretKey, cfg.StripePublishableKey, cfg.StripeWebhookSecret, cfg.PlatformFeeBPS, logger)

	// Log Stripe configuration status (never log actual keys)
	logger.Info("stripe config",
		"secret_key_set", cfg.StripeSecretKey != "",
		"publishable_key_set", cfg.StripePublishableKey != "",
		"webhook_secret_set", cfg.StripeWebhookSecret != "",
		"platform_fee_bps", cfg.PlatformFeeBPS,
	)
	if cfg.StripeWebhookSecret == "" {
		logger.Warn("STRIPE_WEBHOOK_SECRET is empty — webhooks will fail signature verification")
	}

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(userRepo, tokenRepo, jwtSvc, emailSvc, cfg, logger)
	otpAuthHandler := handlers.NewOTPAuthHandler(userRepo, tokenRepo, loginOTPRepo, jwtSvc, otpEmailSvc, logger)
	userHandler := handlers.NewUserHandler(userRepo, docRepo, uploadDir, logger)
	carHandler := handlers.NewCarHandler(carRepo, carPhotoRepo, carDocRepo, userRepo, uploadDir)
	likesHandler := handlers.NewLikesHandler(likesRepo, carRepo)
	chatHandler := handlers.NewChatHandler(chatRepo, uploadDir, wsHub, jwtSvc, logger)
	leaseHandler := handlers.NewLeaseRequestHandler(leaseRepo, carRepo, userRepo, chatRepo, stripeSvc, wsHub, logger)
	todayHandler := handlers.NewTodayHandler(leaseRepo, userRepo, logger)

	// Setup router
	r := chi.NewRouter()

	// Global middleware
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(middleware.Logger(logger))
	r.Use(chiMiddleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Health(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("unhealthy"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Public listings endpoint (for drivers to browse available cars)
		r.Get("/listings", carHandler.ListAvailableListings)

		// Auth routes (public)
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/verify-email", authHandler.VerifyEmail)
			r.Post("/login", authHandler.Login)
			r.Post("/token/refresh", authHandler.RefreshToken)
			r.Post("/password/forgot", authHandler.ForgotPassword)
			r.Post("/password/reset", authHandler.ResetPassword)
			r.Post("/logout", authHandler.Logout)
			r.Post("/resend-otp", authHandler.ResendOTP)

			// OTP email login (passwordless)
			r.Post("/otp/request", otpAuthHandler.RequestOTP)
			r.Post("/otp/verify", otpAuthHandler.VerifyOTP)
			r.Post("/otp/complete-registration", otpAuthHandler.CompleteRegistration)
		})

		// WebSocket endpoint (auth via query param, not middleware)
		r.Get("/ws", chatHandler.HandleWebSocket)

		// Stripe webhook (no auth — verified via signature)
		r.Post("/stripe/webhook", leaseHandler.HandleWebhook)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthMiddleware(jwtSvc))
			r.Get("/me", userHandler.GetCurrentUser)
			r.Patch("/profile", userHandler.UpdateProfile)

			// Profile photo
			r.Post("/profile/photo", userHandler.UploadProfilePhoto)

			// Documents
			r.Get("/documents", userHandler.GetDocuments)
			r.Post("/documents/{type}", userHandler.UploadDocument)
			r.Delete("/documents/{id}", userHandler.DeleteDocument)

			// Onboarding
			r.Post("/onboarding/complete", userHandler.CompleteOnboarding)

			// Actions (Today tab) — chat requests
			r.Get("/me/actions", chatHandler.GetMyActions)

			// Today tab — lease request actions + seen marker
			r.Get("/today/actions", todayHandler.GetActions)
			r.Post("/today/actions/seen", todayHandler.MarkActionsSeen)

			// Likes/Favorites
			r.Get("/me/likes", likesHandler.GetLikedListings)
			r.Post("/listings/{listingId}/like", likesHandler.LikeListing)
			r.Delete("/listings/{listingId}/like", likesHandler.UnlikeListing)

			// Cars
			r.Route("/cars", func(r chi.Router) {
				r.Get("/", carHandler.ListCars)
				r.Post("/", carHandler.CreateCar)
				r.Get("/{carId}", carHandler.GetCar)
				r.Put("/{carId}", carHandler.UpdateCar)
				r.Delete("/{carId}", carHandler.DeleteCar)
				r.Post("/{carId}/pause", carHandler.PauseCar)
				r.Put("/{carId}/location", carHandler.UpdateCarLocation)

				// Car photos
				r.Get("/{carId}/photos", carHandler.ListCarPhotos)
				r.Post("/{carId}/photos", carHandler.UploadCarPhoto)
				r.Delete("/{carId}/photos/{photoId}", carHandler.DeleteCarPhoto)

				// Car documents
				r.Get("/{carId}/documents", carHandler.ListCarDocuments)
				r.Post("/{carId}/documents", carHandler.UploadCarDocument)
				r.Delete("/{carId}/documents/{docId}", carHandler.DeleteCarDocument)
			})

			// Chats
			r.Route("/chats", func(r chi.Router) {
				r.Get("/", chatHandler.ListChats)
				r.Post("/", chatHandler.FindOrCreateChat)
				r.Get("/{chatId}", chatHandler.GetChat)
				r.Get("/{chatId}/messages", chatHandler.ListMessages)
				r.Post("/{chatId}/messages", chatHandler.SendMessage)
				r.Post("/{chatId}/read", chatHandler.MarkRead)
				r.Get("/{chatId}/requests", chatHandler.ListRequests)
				r.Post("/{chatId}/requests", chatHandler.CreateRequest)
				r.Post("/{chatId}/requests/{requestId}/respond", chatHandler.RespondToRequest)
				r.Get("/{chatId}/details", chatHandler.GetChatDetails)
				r.Patch("/{chatId}/settings", chatHandler.UpdateSettings)
				r.Post("/{chatId}/archive", chatHandler.ArchiveChat)
				r.Get("/{chatId}/attachments", chatHandler.ListAttachments)
				r.Post("/{chatId}/attachments", chatHandler.UploadAttachment)
			})

			// User profile (for counterparty profiles in chat)
			r.Get("/users/{userId}/profile", chatHandler.GetUserProfile)

			// Lease requests
			r.Post("/listings/{listingId}/lease-requests", leaseHandler.CreateLeaseRequest)
			r.Get("/chats/{chatId}/lease-requests", leaseHandler.ListLeaseRequests)
			r.Post("/lease-requests/{id}/accept", leaseHandler.AcceptLeaseRequest)
			r.Post("/lease-requests/{id}/decline", leaseHandler.DeclineLeaseRequest)
			r.Post("/lease-requests/{id}/cancel", leaseHandler.CancelLeaseRequest)

			// Payments (Stripe)
			r.Post("/lease-requests/{id}/payments/intent", leaseHandler.CreatePaymentIntent)
			r.Post("/lease-requests/{id}/payments/sync", leaseHandler.SyncPaymentStatus)
		})
	})

	// Serve uploaded files
	fileServer := http.FileServer(http.Dir(uploadDir))
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", fileServer))

	// Serve OpenAPI spec
	r.Get("/openapi", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		spec, err := staticFiles.ReadFile("static/openapi.yaml")
		if err != nil {
			http.Error(w, "OpenAPI spec not found", http.StatusNotFound)
			return
		}
		w.Write(spec)
	})

	// Serve Swagger UI
	r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		html := `<!DOCTYPE html>
<html>
<head>
    <title>DriveBai API - Swagger UI</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui.css">
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui-bundle.js"></script>
    <script>
        window.onload = function() {
            SwaggerUIBundle({
                url: "/openapi",
                dom_id: '#swagger-ui',
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIBundle.SwaggerUIStandalonePreset
                ],
                layout: "BaseLayout"
            });
        }
    </script>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	})

	// Root redirect to docs
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs", http.StatusMovedPermanently)
	})

	// Start server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		logger.Info("starting server", "port", cfg.Port, "env", cfg.Env)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
	}

	logger.Info("server stopped")
}

func init() {
	// Ensure static directory exists for embed
	_ = staticFiles
	fmt.Println("DriveBai API Server")
}
