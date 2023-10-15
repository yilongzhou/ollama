#import <CoreServices/CoreServices.h>
#import "AppDelegate.h"
#import "app.h"

@interface AppDelegate ()

@property (strong, nonatomic) NSStatusItem *statusItem;

@end

@implementation AppDelegate

- (void)applicationDidFinishLaunching:(NSNotification *)aNotification {
    self.statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
    self.statusItem.menu = [self createMenu];
    [self updateImageForAppearance:self.statusItem.button.effectiveAppearance];
    [self.statusItem addObserver:self forKeyPath:@"button.effectiveAppearance" options:NSKeyValueObservingOptionNew|NSKeyValueObservingOptionInitial context:nil];
}

-(void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary<NSKeyValueChangeKey,id> *)change context:(void *)context {
    if ([keyPath isEqualToString:@"button.effectiveAppearance"]) {
        NSStatusItem *item = object;
        [self updateImageForAppearance:item.button.effectiveAppearance];
    }
}

-(void)updateImageForAppearance:(NSAppearance *)appearance {
    NSString *appearanceName = (NSString *)(appearance.name);
    NSString *imageName = ([[appearanceName lowercaseString] containsString:@"dark"]) ? @"iconDark" : @"icon";
    NSImage *statusImage = [NSImage imageNamed:imageName];
    [statusImage setTemplate:YES];
    self.statusItem.button.image = statusImage;
}

- (NSMenu *)createMenu {
    NSMenu *menu = [[NSMenu alloc] init];
    [menu addItemWithTitle:@"Quit Ollama" action:@selector(quit) keyEquivalent:@"q"];
    return menu;
}

- (void)quit {
    Quit();
}

@end
