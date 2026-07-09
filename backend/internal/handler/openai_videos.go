package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func (h *OpenAIGatewayHandler) Videos(c *gin.Context) {
	streamStarted := false
	requestStart := time.Now()

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.openai_gateway.videos",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	isGET := c.Request.Method == http.MethodGet
	endpoint := openAIVideoEndpoint(c)
	var taskID string
	if isGET {
		taskID = extractVideoTaskID(endpoint)
	}

	body, contentType, reqModel, err := readOpenAIVideoGatewayRequest(c)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	if isGET {
		// GET 查询进度 / 取内容：上游 GET 本不需要 model，
		// 从创建时写入的绑定恢复 model（同时粘性命中创建账号）。
		if taskID == "" {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "video task id is required")
			return
		}
		model, ok := h.gatewayService.GetVideoTaskModel(c.Request.Context(), apiKey.GroupID, taskID, apiKey.BundleSubscriptionID, &apiKey.UserID)
		if !ok {
			h.errorResponse(c, http.StatusNotFound, "invalid_request_error", "video task session expired or not found, please recreate the task")
			return
		}
		reqModel = model
	} else {
		if reqModel == "" {
			reqModel = strings.TrimSpace(c.Query("model"))
		}
		if reqModel == "" {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
			return
		}
	}

	reqLog = reqLog.With(zap.String("model", reqModel))
	setOpsRequestContext(c, reqModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))

	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, reqModel)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("openai.videos.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	failedAccountIDs := make(map[int64]struct{})
	var lastFailoverErr *service.UpstreamFailoverError
	switchCount := 0
	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	sessionHash := ""
	if isGET {
		sessionHash = service.VideoTaskSessionHash(taskID)
	}
	routingStart := time.Now()
	for {
		selection, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(),
			apiKey.GroupID,
			"",
			sessionHash,
			reqModel,
			failedAccountIDs,
			service.OpenAIUpstreamTransportHTTPSSE,
			"",
			false,
		)
		if err != nil {
			reqLog.Warn("openai.videos.account_select_failed", zap.Error(err), zap.Int("excluded_account_count", len(failedAccountIDs)))
			if len(failedAccountIDs) == 0 {
				markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available compatible accounts")
				return
			}
			if lastFailoverErr != nil {
				h.handleFailoverExhausted(c, lastFailoverErr, false)
			} else {
				h.errorResponse(c, http.StatusBadGateway, "api_error", "Upstream request failed")
			}
			return
		}
		if selection == nil || selection.Account == nil {
			markOpsRoutingCapacityLimited(c)
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available compatible accounts")
			return
		}
		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)

		accountReleaseFunc, accountAcquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, "", selection, false, &streamStarted, reqLog)
		if !accountAcquired {
			return
		}
		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()
		writerSizeBeforeForward := c.Writer.Size()
		result, err := func() (*service.OpenAIForwardResult, error) {
			defer func() {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
			}()
			return h.gatewayService.ForwardVideos(c.Request.Context(), c, account, c.Request.Method, endpoint, body, contentType, reqModel, channelMapping.MappedModel)
		}()
		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
		}
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)

		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				if c.Writer.Size() != writerSizeBeforeForward {
					h.handleFailoverExhausted(c, failoverErr, true)
					return
				}
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
				h.gatewayService.RecordOpenAIAccountSwitch()
				failedAccountIDs[account.ID] = struct{}{}
				lastFailoverErr = failoverErr
				if switchCount >= maxAccountSwitches {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				switchCount++
				continue
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
			if c.Writer.Size() == writerSizeBeforeForward {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
			}
			reqLog.Warn("openai.videos.forward_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			return
		}

		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, nil)

		if err == nil && result != nil {
			if isGET {
				// GET 查询：到达终态或内容已取回则清理绑定
				isContent := strings.HasSuffix(strings.TrimRight(endpoint, "/"), "/content")
				if service.IsTerminalOpenAIVideosTaskStatus(result.TaskStatus) || isContent {
					h.gatewayService.UnbindVideoTask(c.Request.Context(), apiKey.GroupID, taskID)
				}
			} else if strings.TrimSpace(result.TaskID) != "" {
				// POST 创建成功：写入 task→{account,model} 绑定 + 粘性账号
				if bindErr := h.gatewayService.BindVideoTask(c.Request.Context(), apiKey.GroupID, result.TaskID, account.ID, reqModel, apiKey.BundleSubscriptionID, &apiKey.UserID, service.VideoTaskBindingTTL); bindErr != nil {
					reqLog.Warn("openai.videos.bind_task_failed", zap.Error(bindErr), zap.String("task_id", result.TaskID))
				}
			}
		}

		if c.Request.Method == http.MethodPost && result != nil {
			userAgent := canvasUsageUserAgent(c.GetHeader("User-Agent"), c.GetHeader("X-Canvas-Source"))
			clientIP := ip.GetClientIP(c)
			inboundEndpoint := GetInboundEndpoint(c)
			upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
			h.submitOpenAIUsageRecordTask(c.Request.Context(), result, func(ctx context.Context) {
				if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
					Result:             result,
					APIKey:             apiKey,
					User:               apiKey.User,
					Account:            account,
					Subscription:       subscription,
					InboundEndpoint:    inboundEndpoint,
					UpstreamEndpoint:   upstreamEndpoint,
					UserAgent:          userAgent,
					IPAddress:          clientIP,
					RequestPayloadHash: service.HashUsageRequestPayload(body),
					APIKeyService:      h.apiKeyService,
					ChannelUsageFields: channelMapping.ToUsageFields(reqModel, result.UpstreamModel),
				}); err != nil {
					logger.L().With(zap.String("component", "handler.openai_gateway.videos"), zap.Int64("user_id", subject.UserID), zap.Int64("api_key_id", apiKey.ID), zap.String("model", reqModel), zap.Int64("account_id", account.ID)).Error("openai.videos.record_usage_failed", zap.Error(err))
				}
			})
		}
		return
	}
}

func readOpenAIVideoGatewayRequest(c *gin.Context) ([]byte, string, string, error) {
	if c.Request.Method == http.MethodGet {
		return nil, "", strings.TrimSpace(c.Query("model")), nil
	}
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		return nil, "", "", err
	}
	if len(body) == 0 {
		return nil, "", "", errors.New("Request body is empty") //nolint:staticcheck // message 作为 API 响应返回客户端，与全项目大写约定一致
	}
	contentType := c.GetHeader("Content-Type")
	if strings.HasPrefix(strings.ToLower(contentType), "multipart/form-data") {
		return body, contentType, pkghttputil.ExtractModelFromMultipart(body, contentType), nil
	}
	if !gjson.ValidBytes(body) {
		return nil, "", "", errors.New("Failed to parse request body") //nolint:staticcheck // message 作为 API 响应返回客户端，与全项目大写约定一致
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	return body, contentType, model, nil
}

func canvasUsageUserAgent(userAgent, canvasSource string) string {
	userAgent = strings.TrimSpace(userAgent)
	if strings.TrimSpace(canvasSource) == "" || strings.Contains(strings.ToLower(userAgent), "source=canvas") {
		return userAgent
	}
	if userAgent == "" {
		return "source=canvas"
	}
	return userAgent + " source=canvas"
}

func openAIVideoEndpoint(c *gin.Context) string {
	path := c.Request.URL.Path
	if index := strings.Index(path, "/v1/videos"); index >= 0 {
		return path[index:]
	}
	if index := strings.Index(path, "/videos"); index >= 0 {
		return "/v1" + path[index:]
	}
	return "/v1/videos"
}

// extractVideoTaskID 从 videos endpoint 路径解析 taskID。
// endpoint 形如 "/v1/videos/<id>" 或 "/v1/videos/<id>/content"。
// 当路径不含 task id（如 "/v1/videos/"）时返回空串。
func extractVideoTaskID(endpoint string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimSuffix(trimmed, "/content")
	trimmed = strings.TrimRight(trimmed, "/")
	// 去掉 "/v1/videos" 前缀以拿到 id 段；找不到前缀时退回最后一段。
	prefixIdx := strings.Index(trimmed, "/v1/videos")
	var idSegment string
	if prefixIdx >= 0 {
		idSegment = strings.TrimPrefix(trimmed[prefixIdx:], "/v1/videos")
		idSegment = strings.Trim(idSegment, "/")
	} else {
		idx := strings.LastIndex(trimmed, "/")
		if idx < 0 {
			idSegment = strings.TrimSpace(trimmed)
		} else {
			idSegment = strings.TrimSpace(trimmed[idx+1:])
		}
	}
	return idSegment
}
