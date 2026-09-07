// THROWAWAY. Standard ScriptingBridge creation against scratch Chrome only.
#import <AppKit/AppKit.h>
#import <ScriptingBridge/ScriptingBridge.h>
static void emit(NSDictionary *value) {
    NSData *data=[NSJSONSerialization dataWithJSONObject:value options:NSJSONWritingSortedKeys error:nil];
    puts([[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding].UTF8String);
}
int main(int argc,const char **argv) { @autoreleasepool {
    if(argc!=4)return 2;
    pid_t pid=atoi(argv[1]); NSString *profile=@(argv[2]), *mode=@(argv[3]);
    NSRegularExpression *pattern=[NSRegularExpression regularExpressionWithPattern:@"^/tmp/coupangctl-lifecycle-profile\\.[A-Za-z0-9]+$" options:0 error:nil];
    if(pid<2||[pattern numberOfMatchesInString:profile options:0 range:NSMakeRange(0,profile.length)]!=1||
       ![@[@"make-minimized",@"make-invisible-minimized",@"make-invisible"] containsObject:mode])return 2;
    NSRunningApplication *process=[NSRunningApplication runningApplicationWithProcessIdentifier:pid];
    if(![process.executableURL.path isEqualToString:@"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"])return 2;
    NSTask *task=[NSTask new];NSPipe *pipe=[NSPipe pipe];
    task.executableURL=[NSURL fileURLWithPath:@"/bin/ps"];task.arguments=@[@"-p",@(argv[1]),@"-o",@"command="];
    task.standardOutput=pipe;task.standardError=[NSFileHandle fileHandleWithNullDevice];
    if(![task launchAndReturnError:nil])return 2;
    NSData *bytes=[pipe.fileHandleForReading readDataToEndOfFile];[task waitUntilExit];
    NSString *command=[[NSString alloc]initWithData:bytes encoding:NSUTF8StringEncoding];
    NSString *prefix=@"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome ";
    if(![command hasPrefix:prefix])return 2;
    NSArray *args=[[command substringFromIndex:prefix.length] componentsSeparatedByCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet];
    NSUInteger profileCount=0;
    for(NSString *arg in args) {
        if([arg hasPrefix:@"--user-data-dir="]) { profileCount++;if(![arg isEqualToString:[@"--user-data-dir=" stringByAppendingString:profile]])return 2; }
        if([arg hasPrefix:@"--remote-debugging"]||[arg hasPrefix:@"--headless"]||[arg hasPrefix:@"--load-extension"])return 2;
    }
    if(profileCount!=1||![args containsObject:@"--no-startup-window"])return 2;
    NSString *stage=@"before";
    @try {
        // Apple event reply timeouts use ticks (60 per second).
        SBApplication *app=[SBApplication applicationWithProcessIdentifier:pid];app.timeout=3*60;
        SBElementArray *windows=[app valueForKey:@"windows"];
        if(app.lastError||windows.count!=0){emit(@{@"status":@"unavailable",@"stage":@"before",@"code":@(app.lastError.code)});return 1;}
        NSMutableDictionary *properties=[NSMutableDictionary new];
        if(![mode isEqualToString:@"make-invisible"])properties[@"minimized"]=@YES;
        if(![mode isEqualToString:@"make-minimized"])properties[@"visible"]=@NO;
        stage=@"construct";
        SBObject *window=[[[app classForScriptingClass:@"window"] alloc] initWithProperties:properties];
        stage=@"insert";
        [windows addObject:window];
        if(app.lastError){emit(@{@"status":@"unavailable",@"stage":@"insert",@"code":@(app.lastError.code)});return 1;}
        stage=@"read_state";
        NSMutableArray *states=[NSMutableArray new];
        for(SBObject *w in [app valueForKey:@"windows"]) {
            [states addObject:@{@"minimized":[w valueForKey:@"minimized"],@"visible":[w valueForKey:@"visible"],@"tabs":@([[w valueForKey:@"tabs"] count])}];
        }
        emit(@{@"status":@"ok",@"mode":mode,@"windows":states});
    } @catch(NSException *exception) {emit(@{@"status":@"unavailable",@"stage":stage,@"exception":exception.name});return 1;}
    return 0;
}}
