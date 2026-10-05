// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//
// Portal mínimo del banco FIRe. Hace lo mismo que una aplicación web de una
// Administración integrada con FIRe mediante el cliente Java oficial
// (fire-client-java): crea un lote, añade documentos, pide la firma y, cuando el
// usuario ha firmado, recupera el resultado de cada documento.
//
// Se ejecuta como programa de un solo fichero (java PortalFire.java), con el
// JAR de fire-client-java y sus dependencias en el classpath.
//
// Uso:
//   java PortalFire.java crear <config.properties> <appId> <estado.properties> <urlOk> <urlError> <id=formato=fichero>...
//   java PortalFire.java recuperar <config.properties> <appId> <estado.properties> <dirSalida>
//   java PortalFire.java error <config.properties> <appId> <estado.properties>
//
// Propiedades opcionales:
//   -Dbanco.stopOnError=true   pide a FIRe que detenga el lote en el primer error.
//   -Dbanco.fallosEsperados=a,b documentos que deben fallar; recuperar termina con 0
//                              solo si fallan exactamente esos.

import es.gob.fire.client.BatchResult;
import es.gob.fire.client.CreateBatchResult;
import es.gob.fire.client.FireClient;
import es.gob.fire.client.SignBatchResult;
import es.gob.fire.client.SignOperationResult;
import es.gob.fire.client.TransactionResult;

import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.Base64;
import java.util.Map;
import java.util.Properties;

public final class PortalFire {

    private static final String TITULAR = "99999999R";
    private static final String ALGORITMO = "SHA256withRSA";

    public static void main(final String[] args) throws Exception {
        if (args.length < 4) {
            System.err.println("Uso: crear|recuperar <config> <appId> <estado> ...");
            System.exit(2);
        }
        final Properties config = cargar(Paths.get(args[1]));
        final FireClient cliente = new FireClient(args[2], config);
        final Path estado = Paths.get(args[3]);
        if ("crear".equals(args[0])) {
            crear(cliente, estado, args);
        } else if ("error".equals(args[0])) {
            System.exit(error(cliente, estado));
        } else if ("recuperar".equals(args[0])) {
            System.exit(recuperar(cliente, estado, Paths.get(args[4])));
        } else {
            System.err.println("Operación desconocida: " + args[0]);
            System.exit(2);
        }
    }

    private static void crear(final FireClient cliente, final Path estado, final String[] args) throws Exception {
        final Properties conf = new Properties();
        conf.setProperty("redirectOkUrl", args[4]);
        conf.setProperty("redirectErrorUrl", args[5]);
        conf.setProperty("certOrigin", "local");
        conf.setProperty("appName", "Portal del banco FIRe");
        conf.setProperty("procedureName", "Prueba de firma de lote");

        // Formato por defecto del lote: CAdES. Cada documento puede cambiarlo,
        // como hacen los portales que firman lotes mixtos.
        final Properties extra = new Properties();
        extra.setProperty("mode", "implicit");
        final CreateBatchResult lote = cliente.createBatchProcess(
                TITULAR, "sign", "CAdES", ALGORITMO, propiedadesB64(extra), null, conf);
        final String trId = lote.getTransactionId();

        final StringBuilder ids = new StringBuilder();
        for (int i = 6; i < args.length; i++) {
            final String[] partes = args[i].split("=", 3);
            final String id = partes[0];
            final String formato = partes[1];
            final byte[] datos = Files.readAllBytes(Paths.get(partes[2]));
            final Properties docConf = new Properties();
            docConf.setProperty("docTitle", "Documento " + id);
            docConf.setProperty("docName", Paths.get(partes[2]).getFileName().toString());
            final Properties docExtra = new Properties();
            if ("XAdES".equalsIgnoreCase(formato)) {
                docExtra.setProperty("format", "XAdES Enveloping");
            } else if ("CAdES".equalsIgnoreCase(formato)) {
                docExtra.setProperty("mode", "implicit");
            }
            cliente.addDocumentToBatch(trId, TITULAR, id, datos, docConf, "sign", formato,
                    propiedadesB64(docExtra), null);
            if (ids.length() > 0) {
                ids.append(',');
            }
            ids.append(id);
        }

        final boolean stopOnError = Boolean.getBoolean("banco.stopOnError");
        final SignOperationResult firma = cliente.signBatch(trId, TITULAR, null, stopOnError);
        final Properties salida = new Properties();
        salida.setProperty("transactionId", trId);
        salida.setProperty("redirectUrl", firma.getRedirectUrl());
        salida.setProperty("documentos", ids.toString());
        try (OutputStream out = new FileOutputStream(estado.toFile())) {
            salida.store(out, "Estado del lote del banco FIRe");
        }
        System.out.println(firma.getRedirectUrl());
    }

    private static int recuperar(final FireClient cliente, final Path estado, final Path dirSalida) throws Exception {
        final Properties est = cargar(estado);
        final String trId = est.getProperty("transactionId");
        final BatchResult resultado = cliente.recoverBatchResult(trId, TITULAR);
        System.out.println("Proveedor: " + resultado.getProviderName());
        if (resultado.getSigningCert() != null) {
            System.out.println("Firmante: " + resultado.getSigningCert().getSubjectX500Principal());
        }
        Files.createDirectories(dirSalida);
        final java.util.Set<String> esperados = new java.util.TreeSet<>();
        for (final String e : System.getProperty("banco.fallosEsperados", "").split(",")) {
            if (!e.trim().isEmpty()) {
                esperados.add(e.trim());
            }
        }
        final java.util.Set<String> fallidos = new java.util.TreeSet<>();
        for (final String id : est.getProperty("documentos").split(",")) {
            final SignBatchResult r = resultado.get(id);
            if (r == null || !r.isSigned()) {
                System.out.println("ERROR " + id + ": " + (r == null ? "sin resultado" : r.getErrotType()));
                fallidos.add(id);
                continue;
            }
            final TransactionResult firma = cliente.recoverBatchSign(trId, TITULAR, id);
            final byte[] datos = firma.getResult();
            if (datos == null || datos.length == 0) {
                System.out.println("ERROR " + id + ": firma vacía");
                fallidos.add(id);
                continue;
            }
            Files.write(dirSalida.resolve(id + ".firma"), datos);
            System.out.println("OK " + id + " (" + datos.length + " bytes)");
        }
        if (!fallidos.equals(esperados)) {
            System.out.println("Fallos obtenidos " + fallidos + " distintos de los esperados " + esperados);
            return 1;
        }
        return 0;
    }

    /** Lo que hace un portal cuando FIRe le devuelve a la URL de error. */
    private static int error(final FireClient cliente, final Path estado) throws Exception {
        final Properties est = cargar(estado);
        final TransactionResult r = cliente.recoverErrorResult(est.getProperty("transactionId"), TITULAR);
        System.out.println("Error de FIRe: código " + r.getErrorCode() + ": " + r.getErrorMessage());
        return r.getErrorCode() != 0 ? 0 : 1;
    }

    private static Properties cargar(final Path ruta) throws Exception {
        final Properties p = new Properties();
        try (InputStream in = new FileInputStream(ruta.toFile())) {
            p.load(in);
        }
        return p;
    }

    private static String propiedadesB64(final Properties p) {
        final StringBuilder sb = new StringBuilder();
        for (final Map.Entry<Object, Object> e : p.entrySet()) {
            sb.append(e.getKey()).append('=').append(e.getValue()).append('\n');
        }
        return Base64.getEncoder().encodeToString(sb.toString().getBytes(java.nio.charset.StandardCharsets.UTF_8));
    }
}
