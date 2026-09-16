package pages

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"github.com/Y91R/chromectl/internal/command/session"
)

var screenshotExt = map[string]string{
	"png":  ".png",
	"jpeg": ".jpg",
	"webp": ".webp",
}

type ScreenshotOptions struct {
	// Output — файл; пустой — временный файл, путь к которому возвращается.
	Output   string
	FullPage bool
	Format   string
	Quality  int
	// UID — снять только элемент из последнего снапшота.
	UID string
}

type ScreenshotResult struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	Bytes  int    `json:"bytes"`
}

func (o ScreenshotOptions) validate() error {
	if _, ok := screenshotExt[o.Format]; !ok {
		return fmt.Errorf("формат %s не поддерживается: png, jpeg, webp", o.Format)
	}
	if o.Quality != 0 && o.Format == "png" {
		return errors.New("--quality только для jpeg и webp")
	}
	if o.Quality < 0 || o.Quality > 100 {
		return errors.New("--quality должен быть от 0 до 100")
	}
	if o.UID != "" && o.FullPage {
		return errors.New("--uid и --full-page вместе не используются")
	}
	return nil
}

func Screenshot(ctx context.Context, env Env, opts ScreenshotOptions) (res ScreenshotResult, err error) {
	// Неверные флаги не должны доходить до браузера.
	if err := opts.validate(); err != nil {
		return ScreenshotResult{}, err
	}
	p, err := session.Open(ctx, env, nil)
	if err != nil {
		return ScreenshotResult{}, err
	}
	defer func() {
		_, closeErr := p.Close(ctx)
		err = errors.Join(err, closeErr)
	}()

	params := map[string]any{"format": opts.Format}
	if opts.Quality > 0 {
		params["quality"] = opts.Quality
	}
	switch {
	case opts.FullPage:
		var metrics struct {
			CSSContentSize struct {
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
			} `json:"cssContentSize"`
		}
		if err := p.Client.Call(ctx, p.SessionID, "Page.getLayoutMetrics", nil, &metrics); err != nil {
			return ScreenshotResult{}, err
		}
		params["captureBeyondViewport"] = true
		params["clip"] = map[string]any{
			"x": 0, "y": 0,
			"width":  metrics.CSSContentSize.Width,
			"height": metrics.CSSContentSize.Height,
			"scale":  1,
		}
	case opts.UID != "":
		clip, err := elementClip(ctx, p, opts.UID)
		if err != nil {
			return ScreenshotResult{}, err
		}
		params["clip"] = clip
	}

	var shot struct {
		Data string `json:"data"`
	}
	if err := p.Client.Call(ctx, p.SessionID, "Page.captureScreenshot", params, &shot); err != nil {
		return ScreenshotResult{}, err
	}
	data, err := base64.StdEncoding.DecodeString(shot.Data)
	if err != nil {
		return ScreenshotResult{}, fmt.Errorf("декодирование скриншота: %w", err)
	}

	path, err := writeScreenshot(opts.Output, screenshotExt[opts.Format], data)
	if err != nil {
		return ScreenshotResult{}, err
	}
	return ScreenshotResult{Path: path, Format: opts.Format, Bytes: len(data)}, nil
}

// elementClip переводит рамку элемента из координат окна в координаты страницы,
// которые ждёт clip у Page.captureScreenshot.
func elementClip(ctx context.Context, p *session.Page, uid string) (map[string]any, error) {
	el, err := p.Element(ctx, uid)
	if err != nil {
		return nil, err
	}
	if el.SessionID != p.SessionID {
		return nil, fmt.Errorf("скриншот элемента %s внутри cross-origin iframe не поддерживается: снимите страницу целиком", uid)
	}
	box, err := p.Box(ctx, el)
	if err != nil {
		return nil, err
	}
	var metrics struct {
		CSSVisualViewport struct {
			PageX float64 `json:"pageX"`
			PageY float64 `json:"pageY"`
		} `json:"cssVisualViewport"`
	}
	if err := p.Client.Call(ctx, p.SessionID, "Page.getLayoutMetrics", nil, &metrics); err != nil {
		return nil, err
	}
	return map[string]any{
		"x":      box.X + metrics.CSSVisualViewport.PageX,
		"y":      box.Y + metrics.CSSVisualViewport.PageY,
		"width":  box.Width,
		"height": box.Height,
		"scale":  1,
	}, nil
}

func writeScreenshot(output, ext string, data []byte) (string, error) {
	if output != "" {
		if err := os.WriteFile(output, data, 0o600); err != nil {
			return "", fmt.Errorf("запись скриншота: %w", err)
		}
		return output, nil
	}
	f, err := os.CreateTemp("", "chromectl-*"+ext)
	if err != nil {
		return "", fmt.Errorf("временный файл скриншота: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("запись скриншота: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("запись скриншота: %w", err)
	}
	return f.Name(), nil
}
