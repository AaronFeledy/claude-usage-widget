var tests = new BrowserCookieSnapshotTests();
await tests.Test_ReaderDecryptsNativeDpapiAndAesFixturesOnWindows();
Console.WriteLine("Native DPAPI and AES browser fixtures passed");
