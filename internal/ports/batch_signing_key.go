// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import "context"

// BatchSigningKey es una clave que puede autorizar de una vez las firmas de
// varios trabajos de un lote, como la firma remota CSC cuando el prestador
// admite varias firmas por autorización. El caso de uso de lote calcula
// entonces a la vez los resúmenes de un grupo de trabajos y la clave pide un
// solo PIN u OTP para todos.
type BatchSigningKey interface {
	SigningKey
	// BatchCapacity dice cuántos trabajos de un lote de total se autorizan
	// juntos. 1 o menos significa que se firman uno a uno con la propia
	// clave. Un error impide procesar el lote.
	BatchCapacity(total int) (int, error)
	// BeginBatch prepara la autorización conjunta de n trabajos
	// (2 ≤ n ≤ BatchCapacity).
	BeginBatch(ctx context.Context, n int) (SigningBatch, error)
}

// SigningBatch es la autorización conjunta de un grupo de trabajos. Cada
// trabajo i firma con Key(i), desde su propia goroutine, y llama a Done(i)
// al terminar, haya firmado o no. Close libera la autorización.
type SigningBatch interface {
	Key(i int) SigningKey
	Done(i int)
	Close()
}
