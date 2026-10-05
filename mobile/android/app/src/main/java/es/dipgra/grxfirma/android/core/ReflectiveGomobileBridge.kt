// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package es.dipgra.grxfirma.android.core

import android.content.Context
import es.dipgra.grxfirma.android.BuildConfig
import es.dipgra.grxfirma.android.model.CertificateSummary
import es.dipgra.grxfirma.android.model.LoadedFile
import es.dipgra.grxfirma.android.model.SignedOutput
import es.dipgra.grxfirma.android.model.VerificationSummary
import es.dipgra.grxfirma.android.model.SignatureInspection
import es.dipgra.grxfirma.android.model.BatchItemResult
import es.dipgra.grxfirma.android.model.HashCheck
import es.dipgra.grxfirma.android.model.HashOutput
import es.dipgra.grxfirma.android.model.ProtectionRequest
import java.lang.reflect.InvocationTargetException
import java.lang.reflect.Method
import java.lang.reflect.Proxy
import java.util.Base64

class ReflectiveGomobileBridge private constructor(
    private val facade: Any,
    private val methods: Map<String, Method>,
    override val engineVersion: String,
    private val outputName: (String, String) -> String,
    override val toolsAvailable: Boolean = false,
) : CoreBridge, ExternalIdentityBridge {
    override val readiness = CoreReadiness(
        available = true,
        code = "ready",
        detail = "Contrato mobile v${CoreJsonCodec.CONTRACT_VERSION} y servicios criptográficos verificados.",
    )

    override fun selectCertificate(): CertificateSummary = CoreJsonCodec.parseCertificate(
        invokeJson("selectCertificateJSON", CoreJsonCodec.selectCertificateRequest()),
    )

    override fun importCertificate(data: ByteArray, password: CharArray): CertificateSummary {
        var encoded: ByteArray? = null
        return try {
            encoded = SecretEncoding.utf8(password)
            CoreJsonCodec.parseCertificate(invokeJson("importCertificateSecretBytesJSON", data, encoded))
        } finally {
            encoded?.fill(0)
            password.fill('\u0000')
            data.fill(0)
        }
    }

    override fun installExternalIdentity(
        certificate: ByteArray,
        chain: List<ByteArray>,
        signDigest: (ByteArray, String) -> ByteArray,
    ): CertificateSummary {
        val callbackType = Class.forName("mobilebind.ExternalDigestSigner")
        val callback = Proxy.newProxyInstance(callbackType.classLoader, arrayOf(callbackType)) { proxy, method, args ->
            when (method.name) {
                "signDigest" -> signDigest(args!![0] as ByteArray, args[1] as String)
                "toString" -> "DNIe external signer"
                "hashCode" -> System.identityHashCode(proxy)
                "equals" -> proxy === args?.get(0)
                else -> throw UnsupportedOperationException()
            }
        }
        val payload = org.json.JSONObject()
            .put("certificate_base64", Base64.getEncoder().encodeToString(certificate))
            .put("chain_base64", org.json.JSONArray(chain.map { Base64.getEncoder().encodeToString(it) }))
            .toString()
        return CoreJsonCodec.parseCertificate(invokeJson("installExternalIdentityJSON", payload, callback))
    }

    override fun sign(
        document: LoadedFile,
        format: String,
        certificateId: String,
        options: Map<String, String>,
        action: String,
    ): SignedOutput =
        CoreJsonCodec.parseSigned(
            invokeJson("signJSON", CoreJsonCodec.signRequest(document, format, certificateId, options, action)),
            document.displayName,
            outputName,
        )

    override fun sealPreview(certificateId: String, options: Map<String, String>): ByteArray =
        CoreJsonCodec.parseSealPreview(
            invokeJson("sealPreviewJSON", CoreJsonCodec.sealPreviewRequest(certificateId, options)),
        )

    override fun verify(document: LoadedFile, original: LoadedFile?): VerificationSummary = CoreJsonCodec.parseVerification(
        invokeJson("verifyJSON", CoreJsonCodec.verifyRequest(document, original)),
    )

    override fun inspectSignature(document: LoadedFile): SignatureInspection {
        val json = org.json.JSONObject(invokeJson("inspectSignatureJSON", CoreJsonCodec.verifyRequest(document)))
        val format = json.optString("format").lowercase().takeIf { it in listOf("cades", "pades", "xades") }.orEmpty()
        return SignatureInspection(json.optBoolean("has_signature", false), format)
    }

    override fun createHash(document: LoadedFile, algorithm: String, format: String): HashOutput =
        CoreJsonCodec.parseHash(invokeTool("createHashJSON", CoreJsonCodec.hashRequest(document, algorithm, format)))

    override fun checkHash(document: LoadedFile, hashFile: LoadedFile): HashCheck =
        CoreJsonCodec.parseHashCheck(invokeTool("checkHashJSON", CoreJsonCodec.hashCheckRequest(document, hashFile)))

    override fun protect(document: LoadedFile, request: ProtectionRequest, secret: CharArray?): SignedOutput {
        var encoded = ByteArray(0)
        try {
            if (secret != null && secret.isNotEmpty()) encoded = SecretEncoding.ascii(secret)
            return CoreJsonCodec.parseFileOutput(
                invokeTool("protectJSON", CoreJsonCodec.protectRequest(document, request), encoded),
                "protección",
                document.displayName,
            )
        } finally {
            encoded.fill(0)
            secret?.fill('\u0000')
        }
    }

    override fun unprotect(document: LoadedFile, secret: CharArray?): SignedOutput {
        var encoded = ByteArray(0)
        try {
            if (secret != null && secret.isNotEmpty()) encoded = SecretEncoding.ascii(secret)
            return CoreJsonCodec.parseFileOutput(
                invokeTool("unprotectJSON", CoreJsonCodec.unprotectRequest(document), encoded),
                "desprotección",
                document.displayName,
            )
        } finally {
            encoded.fill(0)
            secret?.fill('\u0000')
        }
    }

    override fun signBatch(
        documents: List<LoadedFile>,
        format: String,
        certificateId: String,
        options: Map<String, String>,
    ): List<BatchItemResult> = CoreJsonCodec.parseBatch(
        invokeTool("processBatchJSON", CoreJsonCodec.batchRequest(documents, format, certificateId, options)),
        documents,
        outputName,
    )

    private fun invokeTool(name: String, vararg payload: Any): String {
        if (!toolsAvailable) throw CoreUnavailableException("TOOLS_UNAVAILABLE")
        return invokeJson(name, *payload)
    }

    override fun clearSession() {
        val method = methods["clearSession"]
            ?: throw CoreContractException("Método de limpieza de sesión no enlazado.")
        invoke(method, facade)
    }

    private fun invokeJson(name: String, payload: String): String {
        val method = methods[name] ?: throw CoreContractException("Método '$name' no enlazado.")
        return invoke(method, facade, payload) as? String
            ?: throw CoreContractException("El método '$name' no devolvió texto JSON.")
    }

    private fun invokeJson(name: String, vararg payload: Any): String {
        val method = methods[name] ?: throw CoreContractException("Método '$name' no enlazado.")
        return invoke(method, facade, *payload) as? String
            ?: throw CoreContractException("El método '$name' no devolvió texto JSON.")
    }

    companion object {
        private const val FACTORY_CLASS = "mobilebind.Mobilebind"
        private const val FACADE_CLASS = "mobilebind.Facade"

        fun create(context: Context): CoreBridge {
            if (BuildConfig.CORE_MODE != "production") {
                return UnavailableCoreBridge(
                    CoreReadiness(
                        available = false,
                        code = "verification_build",
                        detail = "Compilación verificable sin servicios criptográficos.",
                    ),
                )
            }
            return try {
                createProduction(context)
            } catch (error: ReflectiveOperationException) {
                unavailable("core_contract_error", error)
            } catch (error: CoreContractException) {
                unavailable("core_contract_error", error)
            } catch (error: RuntimeException) {
                unavailable("core_runtime_error", error)
            } catch (error: LinkageError) {
                unavailable("core_link_error", error)
            }
        }

        private fun createProduction(context: Context): CoreBridge {
            val factoryClass = Class.forName(FACTORY_CLASS)
            val facadeClass = Class.forName(FACADE_CLASS)
            val factory = factoryClass.getMethod(
                "newAndroidFacade",
                String::class.java,
                String::class.java,
            )
            val facade = invoke(
                factory,
                null,
                context.filesDir.absolutePath,
                context.noBackupFilesDir.absolutePath,
            ) ?: throw CoreContractException("La fábrica Android devolvió una fachada vacía.")
            if (!facadeClass.isInstance(facade)) {
                throw CoreContractException("La fábrica Android devolvió un tipo incompatible.")
            }

            val contractMethod = facadeClass.getMethod("mobileContractJSON")
            val contract = invoke(contractMethod, facade) as? String
                ?: throw CoreContractException("El AAR no devolvió su contrato operativo.")
            CoreJsonCodec.parseContract(contract)

            val required = listOf(
                "selectCertificateJSON",
                "signJSON",
                "sealPreviewJSON",
                "verifyJSON",
                "inspectSignatureJSON",
            ).associateWith { facadeClass.getMethod(it, String::class.java) }
                .toMutableMap()
                .apply {
                    put("installExternalIdentityJSON", facadeClass.getMethod(
                        "installExternalIdentityJSON", String::class.java,
                        Class.forName("mobilebind.ExternalDigestSigner"),
                    ))
                    put("clearSession", facadeClass.getMethod("clearSession"))
                    put(
                        "importCertificateSecretBytesJSON",
                        facadeClass.getMethod(
                            "importCertificateSecretBytesJSON",
                            ByteArray::class.java,
                            ByteArray::class.java,
                        ),
                    )
                }
            // Las herramientas son opcionales: un AAR anterior sigue firmando y
            // verificando, pero no ofrece huellas, protección ni lote.
            val tools = listOf(
                "processBatchJSON" to arrayOf<Class<*>>(String::class.java),
                "createHashJSON" to arrayOf<Class<*>>(String::class.java),
                "checkHashJSON" to arrayOf<Class<*>>(String::class.java),
                "protectJSON" to arrayOf(String::class.java, ByteArray::class.java),
                "unprotectJSON" to arrayOf(String::class.java, ByteArray::class.java),
            ).mapNotNull { (name, types) ->
                try { name to facadeClass.getMethod(name, *types) } catch (_: NoSuchMethodException) { null }
            }.toMap()
            required.putAll(tools)
            val toolsAvailable = tools.size == 5 && CoreJsonCodec.toolsDeclared(contract)
            return ReflectiveGomobileBridge(
                facade,
                required,
                CoreJsonCodec.engineVersion(contract),
                { base, extension ->
                    context.getString(es.dipgra.grxfirma.android.R.string.signed_document_name, base, extension)
                },
                toolsAvailable,
            )
        }

        private fun unavailable(code: String, error: Throwable): CoreBridge = UnavailableCoreBridge(
            CoreReadiness(
                available = false,
                code = code,
                detail = sanitizeError(error),
            ),
        )

        private fun sanitizeError(error: Throwable): String {
            val cause = if (error is InvocationTargetException) error.targetException else error
            return cause.message
                .orEmpty()
                .filter { it == '\n' || !it.isISOControl() }
                .replace(Regex("(?i)[A-Za-z0-9+/]{80,}={0,2}"), "[dato omitido]")
                .trim()
                .take(400)
                .ifBlank { "No se pudo inicializar el núcleo criptográfico Android." }
        }

        private fun invoke(method: Method, receiver: Any?, vararg args: Any?): Any? = try {
            method.invoke(receiver, *args)
        } catch (error: InvocationTargetException) {
            throw CoreContractException(sanitizeError(error), error.targetException)
        }
    }
}
