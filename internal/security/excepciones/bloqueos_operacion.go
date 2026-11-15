// SPDX-License-Identifier: EUPL-1.2
package excepciones

import (
	"context"
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
