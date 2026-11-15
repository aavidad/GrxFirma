// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package excepciones

import (
	"context"
	"strings"
	"sync"
)

type blockReportKey struct{}
type BlockReport struct {
	mu     sync.Mutex
	blocks map[string]*Block
}

// WithBlockReport acredita bloqueos emitidos por Check, sin interpretar texto
// de documentos, portales o mensajes de error externos.
func WithBlockReport(ctx context.Context) (context.Context, *BlockReport) {
	r := &BlockReport{blocks: map[string]*Block{}}
	return context.WithValue(ctx, blockReportKey{}, r), r
}
func (r *BlockReport) For(id string) *Block {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.blocks[id]
	if b == nil {
		return nil
	}
	copy := *b
	return &copy
}
func reportBlock(ctx context.Context, b *Block) {
	if ctx == nil {
		return
	}
	r, _ := ctx.Value(blockReportKey{}).(*BlockReport)
	if r == nil {
		return
	}
	r.mu.Lock()
	r.blocks[b.ID] = b
	r.mu.Unlock()
}

// ForMessage devuelve únicamente un bloqueo que Check ya emitió en esta operación
// y cuyo texto completo aparece en el mensaje. Sirve a los adaptadores que aún
// guardan Error() como texto; el texto puede identificar un bloqueo real de la
// operación, pero nunca crearlo, y un documento o un certificado no pueden
// añadir uno que Check no haya emitido. Siempre que haya error, use errors.As
// o la causa tipada del resultado (domain.VerificationResult.PolicyError).
func (r *BlockReport) ForMessage(message string) *Block {
	if r == nil || message == "" {
		return nil
	}
	for _, e := range Catalog() {
		b := r.For(e.ID)
		if b == nil {
			continue
		}
		if text := b.Error(); text != "" && strings.Contains(message, text) {
			return b
		}
	}
	return nil
}
