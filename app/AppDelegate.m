#import <CoreServices/CoreServices.h>
#import <AppKit/AppKit.h>
#import <Security/Security.h>
#import "AppDelegate.h"
#import "app.h"

@interface AppDelegate ()

@property (strong, nonatomic) NSStatusItem *statusItem;

@end

@implementation AppDelegate

- (void)applicationDidFinishLaunching:(NSNotification *)aNotification {
    // Ask to move to applications directory
    askToMoveToApplications();

    // Once in the desired directory, offer to create a symlink
    // TODO (jmorganca): find a way to provide more context to the
    // user about what this is doing, and ideally use Touch ID.
    // or add an alias in the current shell environment,
    // which wouldn't require any special privileges
    dispatch_async(dispatch_get_main_queue(), ^{ 
        createSymlinkWithAuthorization();
    });

    // show status menu
    NSMenu *menu = [[NSMenu alloc] init];
    [menu addItemWithTitle:@"Quit Ollama" action:@selector(quit) keyEquivalent:@"q"];
    self.statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
    self.statusItem.menu = menu;
    NSImage *statusImage = [NSImage imageNamed:@"icon"];
    [statusImage setTemplate:YES];
    self.statusItem.button.image = statusImage;
}

- (void)quit {
    Quit();
}

@end

int askToMoveToApplications() {
    NSString *bundlePath = [[NSBundle mainBundle] bundlePath];
    if ([bundlePath hasPrefix:@"/Applications"]) {
        return 0;
    }

    NSAlert *alert = [[NSAlert alloc] init];
    [alert setMessageText:@"Move to Applications?"];
    [alert setInformativeText:@"Ollama works best when run from the Applications directory."];
    [alert addButtonWithTitle:@"Move to Applications"];
    [alert addButtonWithTitle:@"Don't move"];
    
    [NSApp activateIgnoringOtherApps:YES];

    if ([alert runModal] != NSAlertFirstButtonReturn) {
        return 0;
    }
    // move to applications
    NSString *applicationsPath = @"/Applications";
    NSString *newPath = [applicationsPath stringByAppendingPathComponent:@"Ollama.app"];
    NSFileManager *fileManager = [NSFileManager defaultManager];

    // Check if the newPath already exists
    if ([fileManager fileExistsAtPath:newPath]) {
        NSError *removeError = nil;
        [fileManager removeItemAtPath:newPath error:&removeError];
        if (removeError) {
            NSLog(@"Error removing file at %@: %@", newPath, removeError);
            return -1; // or handle the error
        }
    }

    NSError *moveError = nil;
    [fileManager moveItemAtPath:bundlePath toPath:newPath error:&moveError];
    if (moveError) {
        NSLog(@"Error moving file from %@ to %@: %@", bundlePath, newPath, moveError);
        return -1; // or handle the error
    }

    NSLog(@"Opening %@", newPath);
    NSError *error = nil;
    NSWorkspace *workspace = [NSWorkspace sharedWorkspace];
    [workspace launchApplicationAtURL:[NSURL fileURLWithPath:newPath]
                                options:NSWorkspaceLaunchNewInstance | NSWorkspaceLaunchDefault
                        configuration:@{}
                                error:&error];
    return 0;
}

int createSymlinkWithAuthorization() {
    NSString *linkPath = @"/usr/local/bin/ollama";
    NSError *error = nil;

    NSFileManager *fileManager = [NSFileManager defaultManager];
    NSString *symlinkPath = [fileManager destinationOfSymbolicLinkAtPath:linkPath error:&error];
    NSString *bundlePath = [[NSBundle mainBundle] bundlePath];
    NSString *execPath = [[NSBundle mainBundle] executablePath];
    NSString *resPath = [[NSBundle mainBundle] pathForResource:@"ollama" ofType:nil];

    // if the symlink already exists and points to the right place, don't prompt
    if ([symlinkPath isEqualToString:execPath] || [symlinkPath isEqualToString:resPath]) {
        return 0;
    }

    OSStatus status;
    AuthorizationRef authorizationRef;
    status = AuthorizationCreate(NULL, kAuthorizationEmptyEnvironment, kAuthorizationFlagDefaults, &authorizationRef);
    if (status != errAuthorizationSuccess) {
        NSLog(@"Failed to create authorization");
        return -1;
    }

    NSString *appBinaryPath = [[NSBundle mainBundle] executablePath];
    const char *sourcePath = [appBinaryPath UTF8String];

    const char *toolPath = "/bin/ln";
    const char *args[] = {"-s", "-F", sourcePath, "/usr/local/bin/ollama", NULL};
    FILE *pipe = NULL;

    status = AuthorizationExecuteWithPrivileges(authorizationRef, toolPath, kAuthorizationFlagDefaults, (char *const *)args, &pipe);
    if (status != errAuthorizationSuccess) {
        NSLog(@"Failed to create symlink");
        return -1;
    }

    AuthorizationFree(authorizationRef, kAuthorizationFlagDestroyRights);

    return 0;
}
