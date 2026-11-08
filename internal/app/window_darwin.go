package app

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void disableTitlebarDoubleClick(void) {
    // Override only this process. Do not change the user's macOS preference.
    NSUserDefaults *defaults = [NSUserDefaults standardUserDefaults];
    NSMutableDictionary *arguments = [[defaults volatileDomainForName:NSArgumentDomain] mutableCopy];
    if (arguments == nil) arguments = [NSMutableDictionary new];
    [arguments setObject:@"None" forKey:@"AppleActionOnDoubleClick"];
    [defaults setVolatileDomain:arguments forName:NSArgumentDomain];
    [arguments release];
}
*/
import "C"

func configureWindowInteractions() { C.disableTitlebarDoubleClick() }
