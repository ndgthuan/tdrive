package com.wails.app;

import android.content.Context;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.AtomicFile;
import java.io.File;
import java.io.FileNotFoundException;
import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;
import java.security.KeyStore;
import java.util.Arrays;
import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;

/** Native-only keys. Ciphertext is excluded from backup and bound to a model ID. */
final class AISecretStore {
    private static final String ALIAS = "tdrive.ai.v1";
    private static Context context;
    private static native void nativeInit();
    static void initialize(Context value) { context = value.getApplicationContext(); nativeInit(); }

    private static SecretKey key() throws Exception {
        KeyStore store = KeyStore.getInstance("AndroidKeyStore"); store.load(null);
        if (!store.containsAlias(ALIAS)) {
            KeyGenerator generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
            generator.init(new KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build());
            generator.generateKey();
        }
        return (SecretKey)store.getKey(ALIAS, null);
    }

    public static synchronized byte[] invoke(int operation, String id, byte[] input) {
        if (context == null || !id.matches("[a-zA-Z0-9-]{1,128}") || input.length > 65536) return new byte[]{2};
        File directory = new File(context.getNoBackupFilesDir(), "ai-keys");
        AtomicFile file = new AtomicFile(new File(directory, id));
        byte[] plain = null;
        try {
            if (operation == 3) { file.delete(); return new byte[]{0}; }
            Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
            byte[] aad = (ALIAS + ":" + id).getBytes(StandardCharsets.UTF_8);
            if (operation == 1) {
                if (!directory.isDirectory() && !directory.mkdirs()) return new byte[]{2};
                cipher.init(Cipher.ENCRYPT_MODE, key()); cipher.updateAAD(aad);
                byte[] encrypted = cipher.doFinal(input);
                FileOutputStream stream = file.startWrite();
                try { stream.write(cipher.getIV()); stream.write(encrypted); file.finishWrite(stream); }
                catch (Exception error) { file.failWrite(stream); throw error; }
                return new byte[]{0};
            }
            if (file.getBaseFile().length() > 65564) return new byte[]{2};
            byte[] encrypted = file.readFully();
            if (encrypted.length < 28) return new byte[]{2};
            cipher.init(Cipher.DECRYPT_MODE, key(), new GCMParameterSpec(128, Arrays.copyOfRange(encrypted, 0, 12)));
            cipher.updateAAD(aad);
            plain = cipher.doFinal(encrypted, 12, encrypted.length - 12);
            byte[] result = new byte[plain.length + 1]; System.arraycopy(plain, 0, result, 1, plain.length); return result;
        } catch (FileNotFoundException missing) { return new byte[]{1}; }
        catch (Exception error) { return new byte[]{2}; }
        finally { Arrays.fill(input, (byte)0); if (plain != null) Arrays.fill(plain, (byte)0); }
    }
}
