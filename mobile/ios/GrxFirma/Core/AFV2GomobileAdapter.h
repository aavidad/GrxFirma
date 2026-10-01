// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#import <Foundation/Foundation.h>

NS_ASSUME_NONNULL_BEGIN

@interface AFV2CoreCallResult : NSObject

@property(nonatomic, readonly, nullable) NSString *value;
@property(nonatomic, readonly, nullable) NSString *errorCode;
@property(nonatomic, readonly, nullable) NSString *errorMessage;
@property(nonatomic, readonly, getter=isSuccess) BOOL success;

@end

@interface AFV2GomobileAdapter : NSObject

@property(nonatomic, readonly, getter=isAvailable) BOOL available;
@property(nonatomic, readonly) NSString *readinessCode;
@property(nonatomic, readonly) NSString *readinessDetail;

- (instancetype)initWithApplicationSupportDirectory:(NSString *)applicationSupportDirectory
                                   appGroupDirectory:(NSString *)appGroupDirectory
                                 keychainAccessGroup:(NSString *)keychainAccessGroup;

- (AFV2CoreCallResult *)mobileContractJSON;
- (void)clearSession;
- (AFV2CoreCallResult *)selectCertificateJSON:(NSString *)payload;
- (AFV2CoreCallResult *)importCertificateJSON:(NSString *)payload;
- (AFV2CoreCallResult *)signJSON:(NSString *)payload;
- (AFV2CoreCallResult *)verifyJSON:(NSString *)payload;
- (AFV2CoreCallResult *)resolvePlatformProfileJSON;

@end

NS_ASSUME_NONNULL_END
