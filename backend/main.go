package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"wa-assistant/backend/config"
	"wa-assistant/backend/database"
	"wa-assistant/backend/handlers"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

func main() {
	database.Init()
	handlers.ConsolidateAllKnowledge()

	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	services.InitAI()
	services.InitEmbedding()
	services.Go("BackfillEmbeddings", services.BackfillEmbeddings)
	services.InitWA(config.Env("DB_PATH", "./wa-assistant.db"))
	services.SetHandlers(handlers.OnWAMessage, handlers.OnDeviceLinked)
	services.SetOutgoingMessageHandler(handlers.OnWAOwnMessage)
	services.SetHistorySyncHandler(handlers.OnWAHistorySync)
	services.SetHistoryChatStateHandler(handlers.OnWAHistoryChatState)
	services.SetWhatsAppReadStateHandler(handlers.OnWAWhatsAppReadState)
	services.SetLabelHandlers(handlers.OnLabelEdit, handlers.OnLabelAssoc)
	services.SetConnectedHandler(handlers.OnAgentConnected)
	services.SetReceiptHandler(handlers.OnWAReceipt)
	services.SetChatPresenceHandler(handlers.OnWAChatPresence)
	services.SetMessageRevokeHandler(handlers.OnWAMessageRevoked)
	handlers.InitGroupGuard()

	services.Go("StartAgents", handlers.StartAgents)
	services.StartReconnectWatchdogCtx(appCtx, 90*time.Second)
	services.Go("ResumeBroadcasts", handlers.ResumeBroadcasts)
	handlers.CleanupStuckSchedules()
	handlers.CleanupBroadcastJunk()
	handlers.CleanupOrphanAssignments()

	services.Go("SeedShippingCities", services.SeedShippingCities)
	handlers.StartSchedulerCtx(appCtx)
	handlers.StartMediaCleanup(config.EnvInt("MEDIA_RETENTION_DAYS", 30))
	handlers.StartFailedSendRetry(appCtx)
	handlers.StartLoginThrottleSweeper()

	r := gin.Default()
	maxRequestMB := config.EnvInt("MAX_REQUEST_MB", 64)
	staticDir := config.Env("STATIC_DIR", "frontend/dist")
	if _, err := os.Stat(staticDir); err == nil {
		r.Static("/assets", staticDir+"/assets")
		r.StaticFile("/favicon.svg", staticDir+"/favicon.svg")
		r.StaticFile("/icons.svg", staticDir+"/icons.svg")
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(404, gin.H{"error": "not found"})
				return
			}
			c.File(staticDir + "/index.html")
		})
	}
	r.MaxMultipartMemory = int64(config.EnvInt("MAX_MULTIPART_MEMORY_MB", 16)) << 20
	r.Use(handlers.BodySizeLimit(int64(maxRequestMB)<<20), handlers.CORS())

	api := r.Group("/api")
	{
		api.POST("/login", handlers.Login)
		api.GET("/verify-email", handlers.VerifyEmail)
		api.POST("/resend-verification", handlers.ResendVerification)
		api.POST("/forgot-password", handlers.ForgotPassword)
		api.POST("/reset-password", handlers.ResetPassword)
		api.GET("/agents/:id/media/:cid", handlers.ServeMedia)
		api.GET("/agents/:id/profile-picture", handlers.ServeProfilePicture)
		api.GET("/agents/:id/products/:pid/image", handlers.ServeProductImage)
		api.GET("/agents/:id/knowledge/:kid/image", handlers.ServeKnowledgeImage)
		api.GET("/me", handlers.AuthMiddleware(), handlers.Me)
		api.PUT("/profile", handlers.AuthMiddleware(), handlers.UpdateProfile)
		api.PUT("/change-password", handlers.AuthMiddleware(), handlers.ChangePassword)

		v1 := api.Group("/v1", handlers.APIKeyMiddleware())
		{
			v1.POST("/messages", handlers.APISendMessage)
			v1.GET("/messages/:message_id/media", handlers.APIServeMessageMedia)
			v1.GET("/messages/:message_id/analysis", handlers.APIMessageAnalysis)
			v1.POST("/otp/request", handlers.APIRequestOTP)
			v1.POST("/otp/verify", handlers.APIVerifyOTP)
			v1.POST("/check", handlers.APICheckNumber)
			v1.GET("/status", handlers.APIStatus)
			v1.GET("/contacts", handlers.APIListContacts)
			v1.POST("/contacts", handlers.APISaveContact)
			v1.GET("/contacts/:number", handlers.APIGetContact)
			v1.PUT("/contacts/:number", handlers.APIUpdateContact)
			v1.DELETE("/contacts/:number", handlers.APIDeleteContact)
			v1.GET("/groups", handlers.APIListGroups)
			v1.POST("/groups/:jid/messages", handlers.APIGroupSendMessage)
			v1.GET("/chats", handlers.APIListChats)
			v1.GET("/chats/:number/messages", handlers.APIChatMessages)
			v1.GET("/media/:cid", handlers.APIServeMedia)
			v1.POST("/broadcasts", handlers.APICreateBroadcast)
			v1.GET("/broadcasts", handlers.APIListBroadcasts)
			v1.GET("/broadcasts/:id", handlers.APIBroadcastStatus)
			v1.GET("/broadcasts/:id/recipients", handlers.APIBroadcastRecipients)
			v1.POST("/broadcasts/:id/cancel", handlers.APICancelBroadcast)
		}
		api.GET("/settings/api-config", handlers.AuthMiddleware(), handlers.GetAPIConfig)
		api.PUT("/settings/api-config", handlers.AuthMiddleware(), handlers.RequireSuperAdmin(), handlers.SaveAPIConfig)
		api.GET("/settings/embedding-models", handlers.AuthMiddleware(), handlers.RequireSuperAdmin(), handlers.ListEmbeddingModels)
		api.GET("/settings/chat-models", handlers.AuthMiddleware(), handlers.RequireSuperAdmin(), handlers.ListChatModels)
		api.GET("/settings/vision-models", handlers.AuthMiddleware(), handlers.RequireSuperAdmin(), handlers.ListVisionModels)

		auth := api.Group("", handlers.AuthMiddleware(), handlers.CSRouteGuard())
		{
			auth.GET("/wa/status", handlers.GetNumberStatus)
			auth.POST("/wa/connect", handlers.ConnectNumber)
			auth.POST("/wa/logout", handlers.LogoutNumber)
			auth.GET("/handoffs", handlers.ListHandoffs)
			auth.DELETE("/handoffs/:sender", handlers.ResumeHandoff)
			auth.GET("/chat-history", handlers.ChatHistory)
			auth.GET("/settings", handlers.GetSettings)
			auth.PUT("/settings", handlers.UpdateSettings)
			auth.GET("/knowledge", handlers.ListKnowledge)
			auth.POST("/knowledge", handlers.CreateKnowledge)
			auth.POST("/knowledge/generate", handlers.GenerateKnowledge)
			auth.POST("/knowledge/import", handlers.ImportKnowledge)
			auth.PUT("/knowledge/:kid", handlers.UpdateKnowledge)
			auth.DELETE("/knowledge/:kid", handlers.DeleteKnowledge)
			auth.GET("/agents", handlers.ListAgents)
			auth.GET("/agents-status", handlers.AgentStatuses)
			auth.GET("/agent-unread-summary", handlers.AgentUnreadSummary)
			auth.GET("/team/users", handlers.RequireTenantAdmin(), handlers.ListTeamUsers)
			auth.POST("/team/users", handlers.RequireTenantAdmin(), handlers.CreateTeamUser)
			auth.PUT("/team/users/:uid", handlers.RequireTenantAdmin(), handlers.UpdateTeamUser)
			auth.DELETE("/team/users/:uid", handlers.RequireTenantAdmin(), handlers.DeleteTeamUser)
			auth.GET("/team/activity", handlers.RequireTenantAdmin(), handlers.ListCSActivity)
			auth.POST("/agents", handlers.CreateAgent)
			auth.PUT("/agents/:id", handlers.UpdateAgent)
			auth.DELETE("/agents/:id", handlers.DeleteAgent)
			auth.GET("/agents/:id/wa/status", handlers.GetNumberStatus)
			auth.POST("/agents/:id/wa/connect", handlers.ConnectNumber)
			auth.POST("/agents/:id/wa/connect-pairing", handlers.ConnectPairingNumber)
			auth.POST("/agents/:id/wa/logout", handlers.LogoutNumber)
			auth.GET("/agents/:id/api", handlers.GetAPISettings)
			auth.POST("/agents/:id/api/key", handlers.RotateAPIKey)
			auth.DELETE("/agents/:id/api/key", handlers.RevokeAPIKey)
			auth.PUT("/agents/:id/api/webhook", handlers.SaveWebhook)
			auth.POST("/agents/:id/api/webhook-secret", handlers.RotateWebhookSecret)
			auth.POST("/agents/:id/api/webhook/test", handlers.TestWebhook)
			auth.POST("/agents/:id/api/test-message", handlers.TestAPIMessage)
			auth.GET("/agents/:id/handoffs", handlers.ListHandoffs)
			auth.DELETE("/agents/:id/handoffs/:sender", handlers.ResumeHandoff)
			auth.GET("/agents/:id/chat-history", handlers.ChatHistory)
			auth.GET("/agents/:id/settings", handlers.GetSettings)
			auth.PUT("/agents/:id/settings", handlers.UpdateSettings)
			auth.POST("/agents/:id/setup-wizard", handlers.SetupWizard)
			auth.GET("/agents/:id/knowledge", handlers.ListKnowledge)
			auth.POST("/agents/:id/knowledge", handlers.CreateKnowledge)
			auth.POST("/agents/:id/knowledge/generate", handlers.GenerateKnowledge)
			auth.POST("/agents/:id/knowledge/import", handlers.ImportKnowledge)
			auth.PUT("/agents/:id/knowledge/:kid", handlers.UpdateKnowledge)
			auth.DELETE("/agents/:id/knowledge-all", handlers.DeleteAllKnowledge)
			auth.DELETE("/agents/:id/knowledge/:kid", handlers.DeleteKnowledge)
			auth.POST("/agents/:id/crawl", handlers.StartCrawl)
			auth.GET("/agents/:id/crawl", handlers.LatestCrawl)
			auth.GET("/agents/:id/crawl/:jobId", handlers.CrawlStatus)
			auth.POST("/agents/:id/crawl/:jobId/train", handlers.TrainCrawlPages)
			auth.POST("/agents/:id/crawl/:jobId/train/stop", handlers.StopTraining)
			auth.GET("/agents/:id/knowledge-usage", handlers.KnowledgeUsage)
			auth.POST("/agents/:id/persona/regenerate", handlers.RegeneratePersona)
			auth.POST("/agents/:id/test-chat", handlers.TestChat)
			auth.GET("/agents/:id/analytics", handlers.AgentAnalytics)
			auth.GET("/agents/:id/ai-metrics", handlers.AgentAIMetrics)
			auth.GET("/agents/:id/contacts", handlers.InboxContacts)
			auth.GET("/agents/:id/conversation", handlers.InboxConversation)
			auth.GET("/agents/:id/inbox/events", handlers.InboxEvents)
			auth.GET("/agents/:id/inbox/incoming-cursor", handlers.InboxIncomingCursor)
			auth.POST("/agents/:id/inbox/client-debug", handlers.InboxClientDebug)
			auth.GET("/agents/:id/link-preview", handlers.LinkPreview)
			auth.POST("/agents/:id/conversation/read", handlers.MarkInboxConversationRead)
			auth.GET("/agents/:id/history-sync", handlers.GetHistorySyncStatus)
			auth.POST("/agents/:id/history-sync", handlers.RequestHistorySync)
			auth.DELETE("/agents/:id/conversation", handlers.DeleteInboxConversation)
			auth.POST("/agents/:id/inbox/reset", handlers.ResetAgentInbox)
			auth.GET("/agents/:id/conversation/brief", handlers.GetConversationBrief)
			auth.POST("/agents/:id/conversation/brief", handlers.RefreshConversationBrief)
			auth.POST("/agents/:id/send", handlers.InboxSend)
			auth.POST("/agents/:id/send-media", handlers.InboxSendMedia)
			auth.POST("/agents/:id/messages/:cid/analyze", handlers.ReanalyzeInboxImage)
			auth.POST("/agents/:id/typing", handlers.ChatPresence)
			auth.DELETE("/agents/:id/messages/:msgId", handlers.RevokeMessage)
			auth.GET("/agents/:id/auto-replies", handlers.ListAutoReplies)
			auth.POST("/agents/:id/auto-replies", handlers.CreateAutoReply)
			auth.PUT("/agents/:id/auto-replies/:rid", handlers.UpdateAutoReply)
			auth.DELETE("/agents/:id/auto-replies/:rid", handlers.DeleteAutoReply)
			auth.GET("/agents/:id/flow", handlers.GetFlow)
			auth.POST("/agents/:id/flow", handlers.SaveFlow)
		}
	}

	port := config.Env("PORT", "3030")
	server := &http.Server{Addr: ":" + port, Handler: r}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()
	<-appCtx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
}
