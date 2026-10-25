//go:build android && cgo

#include "../thumbnail/android_jni.h"

static tdrive_jni_binding ai_binding = TDRIVE_JNI_BINDING_INIT;
JNIEXPORT void JNICALL Java_com_wails_app_AISecretStore_nativeInit(JNIEnv *env, jclass cls) {
    tdrive_jni_bind(&ai_binding, env, cls, "invoke", "(ILjava/lang/String;[B)[B");
}

static void wipe(JNIEnv *env, jbyteArray bytes) {
    if (!bytes || (*env)->ExceptionCheck(env)) return;
    jsize length = (*env)->GetArrayLength(env, bytes);
    jbyte zeros[256] = {0};
    for (jsize i = 0; i < length; i += 256) {
        (*env)->SetByteArrayRegion(env, bytes, i, length - i < 256 ? length - i : 256, zeros);
    }
}

int tdrive_ai_android_secret(int operation, const char *id, const void *input, size_t size, void **output, size_t *length) {
    *output = NULL; *length = 0;
    if (size > 65536) return 1;
    JavaVM *vm; jclass cls; jmethodID method;
    if (!tdrive_jni_resolve(&ai_binding, &vm, &cls, &method)) return 1;
    int attached = 0;
    JNIEnv *env = tdrive_jni_attach(vm, &attached);
    if (!env) return 1;
    if ((*env)->PushLocalFrame(env, 8) < 0) {
        if ((*env)->ExceptionCheck(env)) (*env)->ExceptionClear(env);
        tdrive_jni_detach(vm, attached); return 1;
    }
    jstring account = (*env)->NewStringUTF(env, id);
    jbyteArray data = (*env)->NewByteArray(env, (jsize)size);
    if (data && size) (*env)->SetByteArrayRegion(env, data, 0, (jsize)size, input);
    if (account && data && !(*env)->ExceptionCheck(env)) {
        jbyteArray result = (*env)->CallStaticObjectMethod(env, cls, method, operation, account, data);
        if (result && !(*env)->ExceptionCheck(env) && (*env)->GetArrayLength(env, result) <= 65537) {
            tdrive_jni_take_bytes(env, result, output, length);
            wipe(env, result);
        }
    }
    wipe(env, data);
    if ((*env)->ExceptionCheck(env)) (*env)->ExceptionClear(env);
    (*env)->PopLocalFrame(env, NULL);
    tdrive_jni_detach(vm, attached);
    return *output ? 0 : 1;
}
