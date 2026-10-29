package crawl

import "strconv"

// Factory helpers for ActionDto. Each helper sets Type and the fields that
// action requires, so callers don't need to remember which fields pair with
// which type.

// ClickAction clicks the element matched by selector.
func ClickAction(selector string) ActionDto {
	return ActionDto{Type: ActionTypeClick, Selector: selector}
}

// WaitAction pauses for the given time in milliseconds.
func WaitAction(milliseconds float64) ActionDto {
	return ActionDto{Type: ActionTypeWait, Milliseconds: milliseconds}
}

// WaitSelectorAction waits until the element matched by selector is visible.
// Zero timeoutMs omits the optional field.
func WaitSelectorAction(selector string, timeoutMs float64) ActionDto {
	return ActionDto{Type: ActionTypeWaitSelector, Selector: selector, Milliseconds: timeoutMs}
}

// ScrollXAction scrolls the window horizontally by pixels.
func ScrollXAction(pixels int) ActionDto {
	return ActionDto{Type: ActionTypeScrollX, Text: strconv.Itoa(pixels), X: float64(pixels)}
}

// ScrollYAction scrolls the window vertically by pixels.
func ScrollYAction(pixels int) ActionDto {
	return ActionDto{Type: ActionTypeScrollY, Text: strconv.Itoa(pixels), Y: float64(pixels)}
}

// ScrollToAction scrolls to the element matched by selector.
func ScrollToAction(selector string) ActionDto {
	return ActionDto{Type: ActionTypeScrollTo, Selector: selector}
}

// FillAction types text into the input matched by selector.
func FillAction(selector, text string) ActionDto {
	return ActionDto{Type: ActionTypeFill, Selector: selector, Text: text}
}

// ExecuteAction runs a JavaScript snippet on the page.
func ExecuteAction(script string) ActionDto {
	return ActionDto{Type: ActionTypeExecute, Execute: script}
}

// ScreenshotAction captures the current viewport.
func ScreenshotAction() ActionDto {
	return ActionDto{Type: ActionTypeScreenshot}
}

// FullScreenshotAction captures the full page.
func FullScreenshotAction() ActionDto {
	return ActionDto{Type: ActionTypeScreenshot, FullScreenshot: true}
}

// ElementScreenshotAction captures the element matched by selector.
func ElementScreenshotAction(selector string) ActionDto {
	return ActionDto{Type: ActionTypeScreenshot, ParticularScreenshot: selector}
}

// WaitForRequestAction waits for a network response whose URL matches
// urlPattern. The service interprets the pattern; zero timeoutMs omits the optional field.
func WaitForRequestAction(urlPattern string, timeoutMs float64) ActionDto {
	return ActionDto{Type: ActionTypeWaitForRequestCompletion, UrlPattern: urlPattern, Milliseconds: timeoutMs}
}
