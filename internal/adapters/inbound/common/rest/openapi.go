// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

func openAPIDocument(certificateAuthEnabled, identidadHabilitada bool) map[string]any {
	document := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   "GrxFirma REST API",
			"version": "v2",
		},
		"paths": map[string]any{
			"/health": map[string]any{
				"get": map[string]any{
					"summary": "Comprueba el estado básico del backend local",
				},
			},
			"/certificates": map[string]any{
				"get": map[string]any{
					"summary": "Lista certificados disponibles con metadatos para UI desktop",
				},
			},
			"/certificates/validate": map[string]any{
				"post": map[string]any{
					"summary":     "Valida vigencia, uso de firma y cadena X.509 contra la confianza del sistema",
					"requestBody": jsonRequestBody(certificateIDRequestSchema()),
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Resultado local de vigencia, uso de firma y confianza X.509",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": certificateValidationResponseSchema(),
								},
							},
						},
					},
				},
			},
			"/certificates/online-check": map[string]any{
				"post": map[string]any{
					"summary":     "Comprueba el estado de revocación online de un certificado del catálogo",
					"requestBody": jsonRequestBody(certificateIDRequestSchema()),
				},
			},
			"/openapi.json": map[string]any{
				"get": map[string]any{
					"summary": "Describe la API REST local",
				},
			},
			"/auth/challenge": map[string]any{
				"post": map[string]any{
					"summary": "Genera un reto para autenticación por certificado",
				},
			},
			"/auth/verify": map[string]any{
				"post": map[string]any{
					"summary": "Verifica la firma del reto y emite sesión temporal",
				},
			},
			"/identity/challenges": map[string]any{
				"post": map[string]any{
					"summary":     "Registra y canonicaliza un reto de identidad-reforzada/v1",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"requestBody": jsonRequestBody(identityChallengeRequestSchema()),
				},
			},
			"/identity/verifications": map[string]any{
				"post": map[string]any{
					"summary":     "Consume y verifica una prueba de identidad-reforzada/v1",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"requestBody": jsonRequestBody(identityVerificationRequestSchema()),
				},
			},
			"/sign": map[string]any{
				"post": map[string]any{
					"summary":     "Firma un documento",
					"requestBody": jsonRequestBody(signRequestSchema()),
				},
			},
			"/sign-batch": map[string]any{
				"post": map[string]any{
					"summary":     "Firma varios documentos con un mismo certificado",
					"requestBody": jsonRequestBody(signBatchRequestSchema()),
				},
			},
			"/hash": map[string]any{
				"post": map[string]any{
					"summary":     "Calcula la huella de un documento o fichero",
					"requestBody": jsonRequestBody(hashRequestSchema()),
				},
			},
			"/hash/check": map[string]any{
				"post": map[string]any{
					"summary":     "Comprueba la integridad de un documento o directorio con un fichero de huella",
					"requestBody": jsonRequestBody(hashCheckRequestSchema()),
				},
			},
			"/protect": map[string]any{
				"post": map[string]any{
					"summary":     "Protege/cifra un documento para uno o varios destinatarios locales",
					"requestBody": jsonRequestBody(protectRequestSchema()),
				},
			},
			"/protect-sign": map[string]any{
				"post": map[string]any{
					"summary":     "Protege y firma un documento en un contenedor CMS SignedAndEnvelopedData",
					"requestBody": jsonRequestBody(protectSignRequestSchema()),
				},
			},
			"/unprotect": map[string]any{
				"post": map[string]any{
					"summary":     "Desprotege/descifra un documento protegido",
					"requestBody": jsonRequestBody(unprotectRequestSchema()),
				},
			},
			"/protection/recipients": map[string]any{
				"get": map[string]any{
					"summary": "Lista destinatarios locales compatibles para protección/cifrado",
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Destinatarios y capacidades de protección disponibles localmente",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": protectionRecipientsResponseSchema(),
								},
							},
						},
					},
				},
			},
			"/protection/recipient/export": map[string]any{
				"get": map[string]any{
					"summary": "Exporta el material público de un destinatario fuerte local o importado",
					"parameters": []map[string]any{{
						"name":        "id",
						"in":          "query",
						"required":    true,
						"description": "Identificador del destinatario fuerte a exportar",
						"schema":      map[string]any{"type": "string"},
					}},
				},
			},
			"/protection/recipient/import": map[string]any{
				"post": map[string]any{
					"summary":     "Importa el material público de un destinatario fuerte",
					"requestBody": jsonRequestBody(protectionRecipientImportRequestSchema()),
				},
			},
			"/verify": map[string]any{
				"post": map[string]any{
					"summary":     "Verifica una firma",
					"requestBody": jsonRequestBody(verifyRequestSchema()),
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Resultado de verificación con contrato legacy en la raíz y resultado rico en result",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": verifyResponseSchema(),
								},
							},
						},
					},
				},
			},
			"/v2/verify": map[string]any{
				"post": map[string]any{
					"summary":     "Dictamen PAdES por firma y revisión (contrato autofirmav2.dictamen-verificacion.v2)",
					"description": "Esquema cerrado 2.0.0 en docs/schema/dictamen-verificacion-v2.schema.json; SHA-256 ef4c621ca6d968061ed63428073bc53c501d41e2f21e357a7f5ad10f39a94dae",
					"requestBody": jsonRequestBody(map[string]any{
						"type": "object", "additionalProperties": false,
						"required": []string{"content_base64"},
						"properties": map[string]any{
							"name":                    map[string]any{"type": "string"},
							"content_base64":          map[string]any{"type": "string"},
							"original_content_base64": map[string]any{"type": "string"},
							"contrato_solicitado":     map[string]any{"type": "string", "enum": []string{"autofirmav2.dictamen-verificacion.v2"}},
						},
					}),
					"responses": map[string]any{"200": map[string]any{"description": "Envoltura con dictamen v2; consulte el esquema cerrado publicado"}},
				},
			},
			"/select-certificate": map[string]any{
				"post": map[string]any{
					"summary": "Selecciona un certificado",
				},
			},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type":   "http",
					"scheme": "bearer",
				},
			},
		},
	}
	if !certificateAuthEnabled {
		paths := document["paths"].(map[string]any)
		delete(paths, "/auth/challenge")
		delete(paths, "/auth/verify")
	}
	if !identidadHabilitada {
		paths := document["paths"].(map[string]any)
		delete(paths, "/identity/challenges")
		delete(paths, "/identity/verifications")
	}
	return document
}

func identityChallengeRequestSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false,
		"required": []string{"contract", "challengeId", "audience", "registeredClient", "purpose", "operation", "tenantContextHash", "sessionBinding", "origin", "consentId", "consentVersion", "policyId", "policyVersion", "nonce", "issuedAt", "expiresAt"},
		"properties": map[string]any{
			"contract":    map[string]any{"type": "string", "enum": []string{"identidad-reforzada/v1"}},
			"challengeId": map[string]any{"type": "string", "minLength": 1, "maxLength": 512},
			"audience":    map[string]any{"type": "string"}, "registeredClient": map[string]any{"type": "string"},
			"purpose": map[string]any{"type": "string"}, "operation": map[string]any{"type": "string"},
			"tenantContextHash": map[string]any{"type": "string"}, "sessionBinding": map[string]any{"type": "string"},
			"origin": map[string]any{"type": "string"}, "consentId": map[string]any{"type": "string"},
			"consentVersion": map[string]any{"type": "string"}, "policyId": map[string]any{"type": "string"},
			"policyVersion": map[string]any{"type": "string"}, "nonce": map[string]any{"type": "string"},
			"issuedAt":  map[string]any{"type": "string", "format": "date-time"},
			"expiresAt": map[string]any{"type": "string", "format": "date-time"},
		},
	}
}

func identityVerificationRequestSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false,
		"required": []string{"contract", "challengeId", "signatureB64", "certificateB64", "chainB64", "format", "signatureAlgorithm", "digestAlgorithm"},
		"properties": map[string]any{
			"contract":    map[string]any{"type": "string", "enum": []string{"identidad-reforzada/v1"}},
			"challengeId": map[string]any{"type": "string"}, "signatureB64": map[string]any{"type": "string"},
			"certificateB64":     map[string]any{"type": "string"},
			"chainB64":           map[string]any{"type": "array", "maxItems": 8, "items": map[string]any{"type": "string"}},
			"format":             map[string]any{"type": "string", "enum": []string{"cades-detached"}},
			"signatureAlgorithm": map[string]any{"type": "string", "enum": []string{"sha256-rsa-pkcs1v15", "sha256-ecdsa"}},
			"digestAlgorithm":    map[string]any{"type": "string", "enum": []string{"sha-256"}},
		},
	}
}

func certificateIDRequestSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"certificate_id"},
		"properties": map[string]any{
			"certificate_id": map[string]any{"type": "string", "minLength": 1},
		},
	}
}

func certificateValidationResponseSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{
			"ok", "valid", "certificate_id", "time_valid",
			"digital_signature_usage", "trusted", "chain_depth", "presented_chain_depth",
		},
		"properties": map[string]any{
			"ok":                      map[string]any{"type": "boolean"},
			"valid":                   map[string]any{"type": "boolean"},
			"certificate_id":          map[string]any{"type": "string"},
			"subject":                 map[string]any{"type": "string"},
			"issuer":                  map[string]any{"type": "string"},
			"fingerprint_sha256":      map[string]any{"type": "string"},
			"not_before":              map[string]any{"type": "string", "format": "date-time"},
			"not_after":               map[string]any{"type": "string", "format": "date-time"},
			"time_valid":              map[string]any{"type": "boolean"},
			"digital_signature_usage": map[string]any{"type": "boolean"},
			"trusted":                 map[string]any{"type": "boolean"},
			"chain_depth":             map[string]any{"type": "integer", "minimum": 0},
			"presented_chain_depth":   map[string]any{"type": "integer", "minimum": 1},
			"issues": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
	}
}

func jsonRequestBody(schema map[string]any) map[string]any {
	return map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": schema,
			},
		},
	}
}

func signRequestSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Petición de firma de un único documento. Puede enviarse contenido Base64 o una ruta local inputPath.",
		"properties": map[string]any{
			"request_id": map[string]any{
				"type":        "string",
				"maxLength":   128,
				"pattern":     `^[A-Za-z0-9._:-]+$`,
				"description": "Identificador opcional de idempotencia. Un valor ya procesado se rechaza para impedir firmas duplicadas.",
			},
			"name":               map[string]any{"type": "string"},
			"content_base64":     map[string]any{"type": "string"},
			"mime_type":          map[string]any{"type": "string"},
			"format":             map[string]any{"type": "string", "enum": []string{"AUTO", "PAdES", "CAdES", "XAdES", "XMLdSig", "ODF", "OOXML", "FacturaE", "ASiC-XAdES"}},
			"action":             map[string]any{"type": "string", "enum": []string{"sign", "cosign", "countersign"}},
			"certificate_id":     map[string]any{"type": "string"},
			"certificateIndex":   map[string]any{"type": "integer", "minimum": 0},
			"inputPath":          map[string]any{"type": "string"},
			"outputPath":         map[string]any{"type": "string"},
			"originalPath":       map[string]any{"type": "string"},
			"overwrite":          map[string]any{"type": "string", "enum": []string{"error", "rename", "force"}},
			"saveToDisk":         map[string]any{"type": "boolean"},
			"returnSignatureB64": map[string]any{"type": "boolean"},
			"strictCompat":       map[string]any{"type": "boolean"},
			"allowInvalidPDF":    map[string]any{"type": "boolean"},
			"options":            signOptionsSchema(),
		},
	}
}

func signBatchRequestSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Firma una tanda de documentos con un mismo certificado. El bloque options es la plantilla global; las options de cada item sobrescriben sus claves, incluida page para una página o rango de sello distinto.",
		"properties": map[string]any{
			"request_id": map[string]any{
				"type":        "string",
				"maxLength":   128,
				"pattern":     `^[A-Za-z0-9._:-]+$`,
				"description": "Identificador opcional de idempotencia del lote. Un valor ya procesado se rechaza para impedir firmas duplicadas.",
			},
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":           map[string]any{"type": "string"},
						"content_base64": map[string]any{"type": "string"},
						"mime_type":      map[string]any{"type": "string"},
						"inputPath":      map[string]any{"type": "string"},
						"outputPath":     map[string]any{"type": "string"},
						"options":        signOptionsSchema(),
					},
				},
			},
			"format":             map[string]any{"type": "string", "enum": []string{"AUTO", "PAdES", "CAdES", "XAdES", "XMLdSig", "ODF", "OOXML", "FacturaE", "ASiC-XAdES"}},
			"action":             map[string]any{"type": "string", "enum": []string{"sign", "cosign", "countersign"}},
			"certificate_id":     map[string]any{"type": "string"},
			"certificateIndex":   map[string]any{"type": "integer", "minimum": 0},
			"overwrite":          map[string]any{"type": "string", "enum": []string{"error", "rename", "force"}},
			"saveToDisk":         map[string]any{"type": "boolean"},
			"returnSignatureB64": map[string]any{"type": "boolean"},
			"strictCompat":       map[string]any{"type": "boolean"},
			"allowInvalidPDF":    map[string]any{"type": "boolean"},
			"options":            signOptionsSchema(),
		},
		"required": []string{"items"},
	}
}

func hashRequestSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Calcula una huella compatible con las utilidades createdigest/checkdigest de AutoFirma 1.9. Puede trabajar con contenido Base64 o con inputPath. Si inputPath es un directorio, genera un manifiesto de hashes XML/TXT/CSV.",
		"properties": map[string]any{
			"name":           map[string]any{"type": "string"},
			"content_base64": map[string]any{"type": "string"},
			"inputPath":      map[string]any{"type": "string"},
			"algorithm":      map[string]any{"type": "string", "enum": []string{"SHA-1", "SHA-256", "SHA-384", "SHA-512"}},
			"format":         map[string]any{"type": "string", "enum": []string{"hex", "base64", "bin", "xml", "txt", "csv"}},
			"outputPath":     map[string]any{"type": "string"},
			"overwrite":      map[string]any{"type": "string", "enum": []string{"error", "rename", "force"}},
			"saveToDisk":     map[string]any{"type": "boolean"},
			"recursive":      map[string]any{"type": "boolean"},
		},
	}
}

func hashCheckRequestSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Comprueba una huella almacenada en formato .hexhash, .hashb64 o .hash. Si inputPath es un directorio, comprueba un manifiesto .hashfiles o .txthashfiles y puede generar un informe .hashreport interoperable con AutoFirma 1.9.",
		"properties": map[string]any{
			"name":                map[string]any{"type": "string"},
			"content_base64":      map[string]any{"type": "string"},
			"inputPath":           map[string]any{"type": "string"},
			"hash_content_base64": map[string]any{"type": "string"},
			"hashPath":            map[string]any{"type": "string"},
			"algorithm":           map[string]any{"type": "string", "enum": []string{"SHA-1", "SHA-256", "SHA-384", "SHA-512"}},
			"recursive":           map[string]any{"type": "boolean"},
			"reportOutputPath":    map[string]any{"type": "string"},
			"overwrite":           map[string]any{"type": "string", "enum": []string{"error", "rename", "force"}},
			"saveReportToDisk":    map[string]any{"type": "boolean"},
		},
	}
}

func verifyRequestSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Verifica un documento firmado. Puede enviarse contenido Base64 o una ruta local inputPath. Para firmas detached puede aportarse el original por original_content_base64 u originalPath.",
		"properties": map[string]any{
			"name":                    map[string]any{"type": "string"},
			"mime_type":               map[string]any{"type": "string"},
			"content_base64":          map[string]any{"type": "string"},
			"original_content_base64": map[string]any{"type": "string"},
			"inputPath":               map[string]any{"type": "string"},
			"originalPath":            map[string]any{"type": "string"},
		},
	}
}

func verifyResponseSchema() map[string]any {
	aspect := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status":  map[string]any{"type": "string", "enum": []string{"unknown", "valid", "invalid", "warning"}},
			"reason":  map[string]any{"type": "string"},
			"details": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ok":       map[string]any{"type": "boolean"},
			"valid":    map[string]any{"type": "boolean"},
			"reason":   map[string]any{"type": "string"},
			"details":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"signers":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"dictamen": dictamenSchema(),
			"result": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"valid":       map[string]any{"type": "boolean"},
					"reason":      map[string]any{"type": "string"},
					"details":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"signers":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"format":      map[string]any{"type": "string"},
					"coverage":    map[string]any{"type": "string"},
					"integrity":   aspect,
					"certificate": aspect,
					"trust":       aspect,
					"signerSummaries": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"id":          map[string]any{"type": "string"},
								"subject":     map[string]any{"type": "string"},
								"issuer":      map[string]any{"type": "string"},
								"fingerprint": map[string]any{"type": "string"},
							},
						},
					},
					"warnings": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"errors":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"evidence": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"type":    map[string]any{"type": "string"},
								"summary": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}
}

func protectRequestSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Protege/cifra un documento usando el perfil indicado y destinatarios locales. El perfil por defecto es compat (RSA-OAEP-SHA256 + AES-256-GCM). En el perfil compat puede elegirse contenedor nativo JSON (.afp), CMS EnvelopedData (.enveloped), CMS AuthEnvelopedData autenticado (.authenveloped.p7m) o CMS EncryptedData (.encrypted.p7m). AuthEnvelopedData usa AES-256-GCM y RSA-OAEP-SHA256/MGF1-SHA256 y requiere destinatarios marcados como authEnvelopedDataCompatible. EncryptedData usa secret_b64 y no requiere destinatarios.",
		"properties": map[string]any{
			"name":               map[string]any{"type": "string"},
			"content_base64":     map[string]any{"type": "string"},
			"mime_type":          map[string]any{"type": "string"},
			"profile":            map[string]any{"type": "string", "enum": []string{"compat", "alto", "compat-rsa-oaep-aes256gcm", "alto-mlkem768-aes256gcm"}},
			"recipient_id":       map[string]any{"type": "string"},
			"recipient_ids":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"secret_b64":         map[string]any{"type": "string", "description": "Clave simétrica transitoria en Base64 para CMS EncryptedData. Debe decodificar 32 bytes para AES-256-GCM."},
			"inputPath":          map[string]any{"type": "string"},
			"outputPath":         map[string]any{"type": "string"},
			"overwrite":          map[string]any{"type": "string", "enum": []string{"error", "rename", "force"}},
			"saveToDisk":         map[string]any{"type": "boolean"},
			"returnProtectedB64": map[string]any{"type": "boolean"},
			"options": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
				"description":          "Opciones neutras de protección. Usa una de las formas canónicas de container para elegir el contenedor en perfil compat.",
				"properties": map[string]any{
					"container": map[string]any{
						"type":        "string",
						"enum":        []string{"json", "cms", "authenvelopeddata", "cms-encrypted"},
						"description": "authenvelopeddata genera CMS AuthEnvelopedData con AES-256-GCM y RSA-OAEP-SHA256/MGF1-SHA256.",
					},
				},
			},
		},
	}
}

func protectSignRequestSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Protege y firma un documento en un contenedor CMS SignedAndEnvelopedData compatible con AutoFirma 1.9. Solo está disponible en perfil compat y requiere destinatarios locales y certificado de remitente. Acepta aliases legacy de SignedAndEnvelopedData en options.container y rechaza explícitamente AuthEnvelopedData, AuthenticatedData, CompressedData, EnvelopedData y EncryptedData en este flujo.",
		"properties": map[string]any{
			"name":           map[string]any{"type": "string"},
			"content_base64": map[string]any{"type": "string"},
			"mime_type":      map[string]any{"type": "string"},
			"profile":        map[string]any{"type": "string", "enum": []string{"compat", "compat-rsa-oaep-aes256gcm"}},
			"recipient_id":   map[string]any{"type": "string"},
			"recipient_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"certificate_id": map[string]any{"type": "string"},
			"certificateIndex": map[string]any{
				"type": "integer",
			},
			"inputPath":          map[string]any{"type": "string"},
			"outputPath":         map[string]any{"type": "string"},
			"overwrite":          map[string]any{"type": "string", "enum": []string{"error", "rename", "force"}},
			"saveToDisk":         map[string]any{"type": "boolean"},
			"returnProtectedB64": map[string]any{"type": "boolean"},
			"options": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
				"description":          "Opciones neutras de protección firmada. options.container admite aliases de SignedAndEnvelopedData y se normaliza internamente; otros modos CMS se rechazan en este endpoint.",
			},
		},
	}
}

func unprotectRequestSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Desprotege/descifra un documento protegido en formato .afp o un sobre CMS interoperable. Se soportan EnvelopedData (.enveloped), AuthEnvelopedData autenticado (.authenveloped.p7m, AES-256-GCM + RSA-OAEP-SHA256/MGF1-SHA256) y EncryptedData (.encrypted.p7m, requiere secret_b64).",
		"properties": map[string]any{
			"name":                 map[string]any{"type": "string"},
			"content_base64":       map[string]any{"type": "string"},
			"mime_type":            map[string]any{"type": "string"},
			"secret_b64":           map[string]any{"type": "string", "description": "Clave simétrica transitoria en Base64 para CMS EncryptedData. Debe decodificar 32 bytes para AES-256-GCM."},
			"inputPath":            map[string]any{"type": "string"},
			"outputPath":           map[string]any{"type": "string"},
			"overwrite":            map[string]any{"type": "string", "enum": []string{"error", "rename", "force"}},
			"saveToDisk":           map[string]any{"type": "boolean"},
			"returnUnprotectedB64": map[string]any{"type": "boolean"},
		},
	}
}

func protectionRecipientsResponseSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"ok", "recipients"},
		"properties": map[string]any{
			"ok": map[string]any{"type": "boolean"},
			"recipients": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"id", "label", "profile", "algorithm", "authEnvelopedDataCompatible"},
					"properties": map[string]any{
						"id":        map[string]any{"type": "string"},
						"label":     map[string]any{"type": "string"},
						"profile":   map[string]any{"type": "string", "enum": []string{"compat", "alto"}},
						"algorithm": map[string]any{"type": "string"},
						"authEnvelopedDataCompatible": map[string]any{
							"type":        "boolean",
							"description": "Indica que el destinatario aporta un certificado X.509 RSA y material público coherente para AuthEnvelopedData; el backend vuelve a validar el material al proteger.",
						},
					},
				},
			},
		},
	}
}

func protectionRecipientImportRequestSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Importa un destinatario fuerte previamente exportado, codificado en Base64.",
		"properties": map[string]any{
			"data_base64": map[string]any{"type": "string"},
		},
		"required": []string{"data_base64"},
	}
}

func signOptionsSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Opciones avanzadas de firma. Para PAdES: metadatos opcionales reason/location/contactInfo. Para sello visible PAdES: visibleSeal=true, page=1|all|1,3-5, visibleSealRectX/Y/W/H en puntos PDF, visibleSealKeepText=true|false, visibleSealImageBase64 opcional y qrContent opcional cuando se usa el sello generado por el backend.",
		"additionalProperties": map[string]any{
			"type": "string",
		},
		"examples": []any{
			map[string]any{
				"reason":              "Firma electrónica avanzada",
				"location":            "Granada",
				"contactInfo":         "correo@ejemplo.es",
				"visibleSeal":         "true",
				"page":                "all",
				"visibleSealRectX":    "40",
				"visibleSealRectY":    "40",
				"visibleSealRectW":    "220",
				"visibleSealRectH":    "90",
				"visibleSealKeepText": "true",
				"qrContent":           "https://verifica.ejemplo/",
			},
			map[string]any{
				"reason":           "Aprobación interna",
				"location":         "Oficina de Software Libre",
				"contactInfo":      "contacto@example.invalid",
				"visibleSeal":      "true",
				"page":             "1,3-5",
				"visibleSealRectX": "60",
				"visibleSealRectY": "36",
				"visibleSealRectW": "260",
				"visibleSealRectH": "84",
				"qrContent":        "EXP:2026/0001",
			},
		},
	}
}

// dictamenSchema documenta el contrato autofirmav2.dictamen-verificacion.v1.
func dictamenSchema() map[string]any {
	aspecto := func(estados ...string) map[string]any {
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"estado": map[string]any{"type": "string", "enum": estados},
				"motivo": map[string]any{"type": "string"},
				"fuente": map[string]any{"type": "string"},
				"fecha":  map[string]any{"type": "string", "format": "date-time"},
			},
		}
	}
	cadena := aspecto("valida", "no_valida", "no_comprobada")
	certificado := aspecto("vigente", "no_vigente", "uso_no_permitido", "no_comprobado")
	revocacion := aspecto("vigente", "revocado", "no_comprobada")
	sello := aspecto("no_presente", "valido", "no_valido", "no_comprobado")
	return map[string]any{
		"type":        "object",
		"description": "Dictamen explícito de verificación autónoma. Solo estado=valida con motivo=verificada acredita la firma.",
		"properties": map[string]any{
			"contrato":                map[string]any{"type": "string", "enum": []string{"autofirmav2.dictamen-verificacion.v1"}},
			"estado":                  map[string]any{"type": "string", "enum": []string{"valida", "no_valida", "indeterminada"}},
			"motivo":                  map[string]any{"type": "string"},
			"formato":                 map[string]any{"type": "string"},
			"comprobadoEn":            map[string]any{"type": "string", "format": "date-time"},
			"integridad":              aspecto("valida", "parcial", "no_valida"),
			"cadena":                  cadena,
			"certificado":             certificado,
			"revocacion":              revocacion,
			"selloTiempo":             sello,
			"vinculoOriginal":         aspecto("acreditado", "no_acreditado", "no_aportado"),
			"huellaFirmadoSHA256":     map[string]any{"type": "string"},
			"huellaOriginalSHA256":    map[string]any{"type": "string"},
			"certificadoHuellaSHA256": map[string]any{"type": "string"},
			"firmantes": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"certificadoHuellaSHA256": map[string]any{"type": "string"},
						"serie":                   map[string]any{"type": "string"},
						"asunto":                  map[string]any{"type": "string"},
						"emisor":                  map[string]any{"type": "string"},
						"cadena":                  cadena,
						"certificado":             certificado,
						"revocacion":              revocacion,
						"selloTiempo":             sello,
					},
				},
			},
			"extensiones": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"revocacionRemota":  map[string]any{"type": "string", "enum": []string{"desactivada", "activa"}},
					"selloTiempoRemoto": map[string]any{"type": "string", "enum": []string{"desactivada", "activa"}},
				},
			},
		},
	}
}
