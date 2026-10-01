// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package trustdialog

import (
	"context"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type FyneTrustDialog struct {
	window fyne.Window
}

func NewFyneTrustDialog(w fyne.Window) *FyneTrustDialog {
	return &FyneTrustDialog{window: w}
}

func (f *FyneTrustDialog) PedirDecision(ctx context.Context, origen string) (DecisionUnicaVez, error) {
	resultado := make(chan DecisionUnicaVez, 1)
	msg := buildPromptMessage(origen)

	headline := newTrustDialogText(msg.Headline, 22, true)
	originLabel := newTrustDialogText(msg.OriginLabel, 16, true)
	originValue := newTrustDialogText(msg.OriginValue, 18, false)
	primary := newTrustDialogText(msg.PrimaryMessage, 18, false)
	risk := newTrustDialogText(msg.RiskMessage, 18, false)
	residentRisk := newTrustDialogText(msg.ResidentRisk, 18, false)
	question := newTrustDialogText(msg.Question, 18, true)

	content := container.NewVBox(
		headline,
		widget.NewCard("", "", container.NewVBox(
			originLabel,
			originValue,
		)),
		widget.NewCard(tt("Qué está intentando hacer"), "", primary),
		widget.NewCard(tt("Riesgo si continúas"), "", container.NewVBox(
			risk,
			residentRisk,
		)),
		question,
	)

	var d *dialog.CustomDialog
	fyne.DoAndWait(func() {
		btnSiempre := widget.NewButton(tt("Confiar siempre"), func() {
			resultado <- ConfiarSiempre
			d.Hide()
		})
		btnEstaVez := widget.NewButton(tt("Confiar esta vez"), func() {
			resultado <- ConfiarEstaVez
			d.Hide()
		})
		btnRechazar := widget.NewButton(tt("Rechazar"), func() {
			resultado <- Rechazar
			d.Hide()
		})
		btnRechazar.Importance = widget.DangerImportance
		btnSiempre.Importance = widget.HighImportance

		botones := container.NewGridWithColumns(3, btnSiempre, btnEstaVez, btnRechazar)
		full := container.NewBorder(nil, botones, nil, nil, content)

		d = dialog.NewCustom(tt("Autorización de firma"), tt("Cerrar"), full, f.window)
		d.Resize(fyne.NewSize(760, 420))
		d.SetOnClosed(func() {
			select {
			case resultado <- Rechazar:
			default:
			}
		})
		d.Show()
	})

	select {
	case <-ctx.Done():
		fyne.Do(func() {
			d.Hide()
		})
		return Rechazar, ctx.Err()
	case dec := <-resultado:
		return dec, nil
	}
}

var _ TrustUIProvider = (*FyneTrustDialog)(nil)

func newTrustDialogText(text string, size float32, bold bool) fyne.CanvasObject {
	lbl := canvas.NewText(wrapTrustDialogText(text, 88), theme.Color(theme.ColorNameForeground))
	lbl.TextSize = size
	lbl.TextStyle = fyne.TextStyle{Bold: bold}
	return lbl
}

func wrapTrustDialogText(text string, width int) string {
	text = strings.TrimSpace(text)
	if text == "" || width < 8 {
		return text
	}
	paragraphs := strings.Split(text, "\n")
	out := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			out = append(out, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := words[0]
		lines := make([]string, 0, 4)
		for _, word := range words[1:] {
			if len([]rune(line))+1+len([]rune(word)) > width {
				lines = append(lines, line)
				line = word
				continue
			}
			line += " " + word
		}
		lines = append(lines, line)
		out = append(out, strings.Join(lines, "\n"))
	}
	return strings.Join(out, "\n")
}
