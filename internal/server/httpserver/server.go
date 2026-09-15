// Package httpserver собирает echo: middleware, валидацию по контракту, роутер
// API и раздачу собранного фронта — монолит отдаёт и API, и UI на одном порту.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	oapimiddleware "github.com/oapi-codegen/echo-middleware"

	api "github.com/your-org/chrome_skill/gen/api"
)

const basePath = "/api/v1"

type Config struct {
	Addr        string
	CORSOrigins []string
	StaticDir   string
}

type Server struct {
	echo *echo.Echo
	addr string
	log  *slog.Logger
}

func New(cfg Config, handlers api.StrictServerInterface, log *slog.Logger) (*Server, error) {
	spec, err := api.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("загрузка спецификации: %w", err)
	}
	// Список серверов не обнуляется: в нём относительный путь /api/v1, по которому
	// валидатор и сопоставляет запрос со спекой.

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	// Таймауты задаются явно: с дефолтными (их нет) клиент, отправляющий заголовки
	// по байту, держит соединение и горутину сколько угодно.
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 30 * time.Second
	e.Server.WriteTimeout = 30 * time.Second
	e.Server.IdleTimeout = 120 * time.Second
	e.HTTPErrorHandler = apiErrorHandler(e, log)

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(requestLogger(log))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: cfg.CORSOrigins,
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderContentType, echo.HeaderAuthorization},
	}))
	// Валидация по контракту до хендлера: неверный запрос не должен доходить
	// до домена и превращаться там в невнятную ошибку. Статика SPA в спеке
	// не описана, поэтому проверяется только /api.
	e.Use(oapimiddleware.OapiRequestValidatorWithOptions(spec, &oapimiddleware.Options{
		SilenceServersWarning: true,
		Skipper:               func(c echo.Context) bool { return !isAPI(c) },
	}))
	// Монолит: собранный фронт (cfg.StaticDir, по умолчанию front/public) раздаётся
	// с диска тем же сервером; HTML5 — SPA-fallback на index.html.
	e.Use(middleware.StaticWithConfig(middleware.StaticConfig{
		Root:    cfg.StaticDir,
		Index:   "index.html",
		HTML5:   true,
		Skipper: isAPI,
	}))

	api.RegisterHandlersWithBaseURL(e, api.NewStrictHandler(handlers, nil), basePath)
	return &Server{echo: e, addr: cfg.Addr, log: log}, nil
}

func isAPI(c echo.Context) bool {
	return strings.HasPrefix(c.Request().URL.Path, "/api/")
}

// Коды ответов ошибок описаны в спеке схемой Error{code,message}, но часть из них
// формируют middleware (валидация по контракту, роутер), а не хендлеры. Без общего
// обработчика клиент получал бы на те же коды формат echo — контракт врал бы.
func apiErrorHandler(e *echo.Echo, log *slog.Logger) echo.HTTPErrorHandler {
	slugs := map[int]string{
		http.StatusBadRequest:       "validation",
		http.StatusUnauthorized:     "unauthorized",
		http.StatusForbidden:        "forbidden",
		http.StatusNotFound:         "not_found",
		http.StatusMethodNotAllowed: "method_not_allowed",
		http.StatusConflict:         "conflict",

		http.StatusInternalServerError: "internal",
	}

	return func(err error, c echo.Context) {
		if !isAPI(c) {
			// SPA отдаётся статикой: там ошибка — это fallback на index.html,
			// а не JSON контракта.
			e.DefaultHTTPErrorHandler(err, c)
			return
		}
		if c.Response().Committed {
			return
		}

		status := http.StatusInternalServerError
		message := "внутренняя ошибка"

		var he *echo.HTTPError
		if errors.As(err, &he) {
			status = he.Code
			if status < http.StatusInternalServerError {
				message = fmt.Sprint(he.Message)
			}
		}

		// Ошибка логируется один раз — здесь, где она обработана; наружу
		// её текст не уходит: он может нести детали хранилища и конфигурации.
		if status >= http.StatusInternalServerError {
			log.Error("запрос завершился ошибкой",
				slog.String("method", c.Request().Method),
				slog.String("uri", c.Request().RequestURI),
				slog.Any("error", err),
			)
		}

		slug, ok := slugs[status]
		if !ok {
			slug = "error"
		}

		if c.Request().Method == http.MethodHead {
			_ = c.NoContent(status)
			return
		}
		if err := c.JSON(status, api.Error{Code: slug, Message: message}); err != nil {
			log.Error("не удалось отправить тело ошибки", slog.Any("error", err))
		}
	}
}

func (s *Server) Run() error {
	s.log.Info("HTTP-сервер запущен", slog.String("addr", s.addr))
	if err := s.echo.Start(s.addr); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error { return s.echo.Shutdown(ctx) }

func requestLogger(log *slog.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:    true,
		LogURI:       true,
		LogMethod:    true,
		LogLatency:   true,
		LogError:     true,
		LogRequestID: true,
		LogValuesFunc: func(_ echo.Context, v middleware.RequestLoggerValues) error {
			log.Info("запрос обработан",
				slog.String("method", v.Method),
				slog.String("uri", v.URI),
				slog.Int("status", v.Status),
				slog.Duration("latency", v.Latency.Round(time.Millisecond)),
				slog.String("request_id", v.RequestID),
			)
			return nil
		},
	})
}
