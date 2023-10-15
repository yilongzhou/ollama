#import <Cocoa/Cocoa.h>
#import "AppDelegate.h"

void run() {
    @autoreleasepool {
        [NSApplication sharedApplication];
        AppDelegate *appDelegate = [[AppDelegate alloc] init];
        [NSApp setDelegate:appDelegate];
        [NSApp run];
    }
}
