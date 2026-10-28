package com.chatv0.agent

import java.io.File
import java.security.MessageDigest
import java.security.cert.CertificateFactory
import org.junit.Assert.*
import org.junit.Test

class TrustAnchorTest {
    @Test fun compatibilityAnchorIsTheOfficialPublicRootNotAnArbitraryCertificate() {
        val candidates = listOf(File("src/main/res/raw/isrg_root_x1.pem"), File("android-agent/app/src/main/res/raw/isrg_root_x1.pem"))
        val file = candidates.first { it.exists() }
        val cert = file.inputStream().use { CertificateFactory.getInstance("X.509").generateCertificate(it) }
        val digest = MessageDigest.getInstance("SHA-256").digest(cert.encoded)
            .joinToString("") { "%02x".format(it.toInt() and 255) }
        assertEquals("96bcec06264976f37460779acf28c5a7cfe8a3c0aae11a8ffcee05c0bddf08c6", digest)
        cert.verify(cert.publicKey)
    }
}
