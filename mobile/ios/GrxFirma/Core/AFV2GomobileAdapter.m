// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#import "AFV2GomobileAdapter.h"

#if GRXFIRMA_PRODUCTION_CORE
#if !__has_include(<Mobilebind/Mobilebind.h>)
#error "Release requiere Frameworks/Mobilebind.xcframework validado"
#endif
#import <Mobilebind/Mobilebind.h>
#endif

@interface AFV2CoreCallResult ()
@property(nonatomic, readwrite, nullable) NSString *value;
@property(nonatomic, readwrite, nullable) NSString *errorCode;
@property(nonatomic, readwrite, nullable) NSString *errorMessage;
+ (instancetype)success:(NSString *)value;
+ (instancetype)failure:(NSString *)code error:(NSError *)error;
@end

@implementation AFV2CoreCallResult

+ (instancetype)success:(NSString *)value {
    AFV2CoreCallResult *result = [[self alloc] init];
    result.value = value;
    return result;
}

+ (instancetype)failure:(NSString *)code error:(NSError *)error {
    AFV2CoreCallResult *result = [[self alloc] init];
    result.errorCode = code;
    NSString *message = error.localizedDescription ?: @"El núcleo no devolvió un error utilizable.";
    result.errorMessage = message.length > 500 ? [message substringToIndex:500] : message;
    return result;
}

- (BOOL)isSuccess {
    return self.value != nil && self.errorCode == nil;
}

@end

@interface AFV2GomobileAdapter ()
@property(nonatomic, readwrite, getter=isAvailable) BOOL available;
@property(nonatomic, readwrite) NSString *readinessCode;
@property(nonatomic, readwrite) NSString *readinessDetail;
#if GRXFIRMA_PRODUCTION_CORE
@property(nonatomic) MobilebindFacade *facade;
#endif
@end

@implementation AFV2GomobileAdapter

- (instancetype)initWithApplicationSupportDirectory:(NSString *)applicationSupportDirectory
                                   appGroupDirectory:(NSString *)appGroupDirectory
                                 keychainAccessGroup:(NSString *)keychainAccessGroup {
    self = [super init];
    if (self == nil) {
        return nil;
    }
#if GRXFIRMA_PRODUCTION_CORE
    NSError *error = nil;
    _facade = MobilebindNewIOSFacade(applicationSupportDirectory,
                                     appGroupDirectory,
                                     keychainAccessGroup,
                                     &error);
    if (_facade == nil || error != nil) {
        _available = NO;
        _readinessCode = @"ios_factory_failed";
        _readinessDetail = [self.class sanitizedMessage:error fallback:@"La fábrica iOS no pudo iniciar el núcleo."];
        return self;
    }
    _available = YES;
    _readinessCode = @"ready";
    _readinessDetail = @"Núcleo criptográfico iOS inicializado.";
#else
    (void)applicationSupportDirectory;
    (void)appGroupDirectory;
    (void)keychainAccessGroup;
    _available = NO;
    _readinessCode = @"verification_build";
    _readinessDetail = @"Compilación de verificación sin núcleo criptográfico enlazado.";
#endif
    return self;
}

+ (NSString *)sanitizedMessage:(NSError *)error fallback:(NSString *)fallback {
    NSString *message = error.localizedDescription ?: fallback;
    NSCharacterSet *controls = [NSCharacterSet controlCharacterSet];
    NSArray<NSString *> *parts = [message componentsSeparatedByCharactersInSet:controls];
    NSString *sanitized = [parts componentsJoinedByString:@" "];
    return sanitized.length > 500 ? [sanitized substringToIndex:500] : sanitized;
}

#if GRXFIRMA_PRODUCTION_CORE
- (AFV2CoreCallResult *)call:(NSString * _Nullable (^)(NSError **))operation code:(NSString *)code {
    if (!self.available || self.facade == nil) {
        NSError *error = [NSError errorWithDomain:@"io.github.aavidad.grxfirma.core"
                                             code:1
                                         userInfo:@{NSLocalizedDescriptionKey: self.readinessDetail}];
        return [AFV2CoreCallResult failure:self.readinessCode error:error];
    }
    NSError *error = nil;
    NSString *value = operation(&error);
    if (value == nil || error != nil) {
        NSError *reported = error ?: [NSError errorWithDomain:@"io.github.aavidad.grxfirma.core"
                                                          code:2
                                                      userInfo:@{NSLocalizedDescriptionKey: @"Respuesta vacía del núcleo."}];
        return [AFV2CoreCallResult failure:code error:reported];
    }
    return [AFV2CoreCallResult success:value];
}
#else
- (AFV2CoreCallResult *)unavailable {
    NSError *error = [NSError errorWithDomain:@"io.github.aavidad.grxfirma.core"
                                         code:3
                                     userInfo:@{NSLocalizedDescriptionKey: self.readinessDetail}];
    return [AFV2CoreCallResult failure:self.readinessCode error:error];
}
#endif

- (AFV2CoreCallResult *)mobileContractJSON {
#if GRXFIRMA_PRODUCTION_CORE
    if (!self.available || self.facade == nil) {
        return [AFV2CoreCallResult failure:self.readinessCode
                                     error:[NSError errorWithDomain:@"io.github.aavidad.grxfirma.core"
                                                               code:4
                                                           userInfo:@{NSLocalizedDescriptionKey: self.readinessDetail}]];
    }
    NSString *value = [self.facade mobileContractJSON];
    if (value == nil || value.length == 0) {
        return [AFV2CoreCallResult failure:@"contract_failed"
                                     error:[NSError errorWithDomain:@"io.github.aavidad.grxfirma.core"
                                                               code:5
                                                           userInfo:@{NSLocalizedDescriptionKey: @"Contrato mobile vacío."}]];
    }
    return [AFV2CoreCallResult success:value];
#else
    return [self unavailable];
#endif
}

- (void)clearSession {
#if GRXFIRMA_PRODUCTION_CORE
    [self.facade clearSession];
#endif
}

- (AFV2CoreCallResult *)selectCertificateJSON:(NSString *)payload {
#if GRXFIRMA_PRODUCTION_CORE
    return [self call:^NSString *(NSError **error) {
        return [self.facade selectCertificateJSON:payload error:error];
    } code:@"select_certificate_failed"];
#else
    (void)payload;
    return [self unavailable];
#endif
}

- (AFV2CoreCallResult *)importCertificateJSON:(NSString *)payload {
#if GRXFIRMA_PRODUCTION_CORE
    return [self call:^NSString *(NSError **error) {
        return [self.facade importCertificateJSON:payload error:error];
    } code:@"import_certificate_failed"];
#else
    (void)payload;
    return [self unavailable];
#endif
}

- (AFV2CoreCallResult *)signJSON:(NSString *)payload {
#if GRXFIRMA_PRODUCTION_CORE
    return [self call:^NSString *(NSError **error) {
        return [self.facade signJSON:payload error:error];
    } code:@"sign_failed"];
#else
    (void)payload;
    return [self unavailable];
#endif
}

- (AFV2CoreCallResult *)verifyJSON:(NSString *)payload {
#if GRXFIRMA_PRODUCTION_CORE
    return [self call:^NSString *(NSError **error) {
        return [self.facade verifyJSON:payload error:error];
    } code:@"verify_failed"];
#else
    (void)payload;
    return [self unavailable];
#endif
}

- (AFV2CoreCallResult *)resolvePlatformProfileJSON {
#if GRXFIRMA_PRODUCTION_CORE
    return [self call:^NSString *(NSError **error) {
        return [self.facade resolvePlatformProfileJSON:error];
    } code:@"profile_failed"];
#else
    return [self unavailable];
#endif
}

@end
