; PureUtils modular installer (per-user, no admin required).
; Components: PureShot (capture/pins) + PureClip (clipboard history) + PureImage (viewer & space preview) + PureVideo (fast video browser) + PureZip (archive manager).
; Build via: .\build.ps1 pack

#ifndef StageDir
  #define StageDir "..\artifacts\stage"
#endif
#ifndef OutputDir
  #define OutputDir "..\artifacts\installer"
#endif
#ifndef AppVersion
  #define AppVersion "1.2.0"
#endif

[Setup]
AppId={{6D9E4A1B-3C82-4F55-9A14-2B7E8C3D5F10}
AppName=PureUtils
AppVersion={#AppVersion}
AppVerName=PureUtils {#AppVersion}
AppPublisher=PureUtils
AppPublisherURL=https://github.com/WangsYi/PureUtils
AppSupportURL=https://github.com/WangsYi/PureUtils/issues
AppUpdatesURL=https://github.com/WangsYi/PureUtils/releases
DefaultDirName={localappdata}\Programs\PureUtils
DefaultGroupName=PureUtils
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
OutputDir={#OutputDir}
OutputBaseFilename=PureUtils-{#AppVersion}-setup
SetupIconFile={#StageDir}\resources\setup.ico
UninstallDisplayIcon={app}\resources\icon.ico
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern dynamic windows11 hidebevels
WizardSizePercent=100,100
WizardBackColor=#f8fafc
WizardBackColorDynamicDark=#18181b
WizardImageFile=
WizardSmallImageFile=
DisableWelcomePage=no
DisableReadyPage=yes
ShowLanguageDialog=auto
MinVersion=10.0
ChangesAssociations=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
CloseApplications=yes
CloseApplicationsFilter=*.exe,*.dll
RestartApplications=no
VersionInfoVersion={#AppVersion}.0
VersionInfoProductName=PureUtils
VersionInfoProductVersion={#AppVersion}

[Languages]
Name: "chinesesimplified"; MessagesFile: "ChineseSimplified.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Messages]
chinesesimplified.SetupWindowTitle=PureUtils
chinesesimplified.BeveledLabel=
chinesesimplified.ClickNext=
chinesesimplified.ClickFinish=
chinesesimplified.ButtonBack=← 返回
chinesesimplified.ButtonNext=下一步(&N)
chinesesimplified.ButtonInstall=安装(&I)
chinesesimplified.ButtonFinish=完成(&F)
chinesesimplified.ButtonCancel=取消
chinesesimplified.ButtonWizardBrowse=浏览…
chinesesimplified.StatusClosingApplications=正在准备程序文件…
chinesesimplified.StatusCreateDirs=正在创建安装目录…
chinesesimplified.StatusExtractFiles=正在释放程序文件…
chinesesimplified.StatusCreateIcons=正在创建快捷方式…
chinesesimplified.StatusCreateRegistryEntries=正在配置系统集成…
chinesesimplified.StatusSavingUninstall=正在写入卸载信息…
chinesesimplified.StatusRunProgram=正在完成安装…
chinesesimplified.ComponentsDiskSpaceMBLabel=所需空间：[mb] MB
chinesesimplified.ComponentsDiskSpaceGBLabel=所需空间：[gb] GB

english.SetupWindowTitle=PureUtils
english.BeveledLabel=
english.ClickNext=
english.ClickFinish=
english.ButtonBack=← Back
english.ButtonNext=&Next
english.ButtonInstall=&Install
english.ButtonFinish=&Finish
english.ButtonCancel=Cancel
english.ButtonWizardBrowse=Browse…
english.StatusClosingApplications=Preparing files…
english.StatusCreateDirs=Creating folders…
english.StatusExtractFiles=Extracting files…
english.StatusCreateIcons=Creating shortcuts…
english.StatusCreateRegistryEntries=Configuring integration…
english.StatusSavingUninstall=Saving uninstall info…
english.StatusRunProgram=Finishing installation…
english.ComponentsDiskSpaceMBLabel=Required space: [mb] MB
english.ComponentsDiskSpaceGBLabel=Required space: [gb] GB

[CustomMessages]
chinesesimplified.HeroTagline=轻量、原生的 Windows 五合一效率工具集
chinesesimplified.HeroStatusNew=版本 {#AppVersion} · 截图 · 剪贴板 · 看图 · 视频 · 压缩
chinesesimplified.HeroStatusUpgrade=将从版本 %1 升级到 {#AppVersion}
chinesesimplified.HeroStatusSame=已安装版本 %1，重新安装可修复或修改组件
chinesesimplified.BtnInstallNow=立即安装
chinesesimplified.BtnUpgradeNow=立即升级
chinesesimplified.BtnReinstall=重新安装
chinesesimplified.BtnStartInstall=开始安装
chinesesimplified.BtnStartUpgrade=开始升级
chinesesimplified.BtnClose=关闭
chinesesimplified.BtnBrowse=浏览…
chinesesimplified.LinkCustomize=自定义选项 ▸
chinesesimplified.LinkBack=← 返回
chinesesimplified.LabelLocation=安装位置：%1
chinesesimplified.LabelCustomTitle=自定义安装
chinesesimplified.LabelInstallDir=安装目录
chinesesimplified.ChooseDirPrompt=选择 PureUtils 的安装文件夹
chinesesimplified.PrepTitle=正在准备安装…
chinesesimplified.PrepChecking=正在检查运行状态…
chinesesimplified.PrepClosing=正在关闭运行中的 %1…
chinesesimplified.ProgressInstalling=正在安装 PureUtils…
chinesesimplified.ProgressUpdating=正在升级 PureUtils…
chinesesimplified.FinishHeading=安装完成
chinesesimplified.FinishBody=PureUtils {#AppVersion} 已准备就绪。
chinesesimplified.TypeFull=完整安装（全部组件）
chinesesimplified.TypeShotClip=截图与剪贴板（PureShot + PureClip）
chinesesimplified.TypeViewer=仅 PureImage 看图
chinesesimplified.TypeVideo=仅 PureVideo 视频
chinesesimplified.TypeZip=仅 PureZip 压缩
chinesesimplified.TypeCustom=自定义
chinesesimplified.CompCore=核心框架（必需）
chinesesimplified.CompCapture=PureShot 截图贴图
chinesesimplified.CompHistory=PureClip 剪贴板历史
chinesesimplified.CompViewer=PureImage 看图与空格预览
chinesesimplified.CompTurbo=JPEG 原生加速 (libjpeg-turbo)
chinesesimplified.CompAssoc=关联图片格式
chinesesimplified.CompVideo=PureVideo 视频播放
chinesesimplified.CompVideoAssoc=关联视频格式
chinesesimplified.CompZip=PureZip 压缩管理
chinesesimplified.CompZipContext=资源管理器右键菜单
chinesesimplified.CompZipAssoc=关联压缩格式
chinesesimplified.TaskDesktopIcon=创建桌面快捷方式
chinesesimplified.TaskAutostart=开机自动启动
chinesesimplified.LaunchMain=启动 PureUtils
chinesesimplified.LaunchViewer=启动 PureImage
chinesesimplified.LaunchVideo=启动 PureVideo
chinesesimplified.LaunchZip=启动 PureZip
chinesesimplified.NeedModule=请至少选择一个功能组件。

english.HeroTagline=A lightweight, native 5-in-1 Windows utility suite
english.HeroStatusNew=Version {#AppVersion} · Shot · Clip · Image · Video · Zip
english.HeroStatusUpgrade=Upgrading from %1 to {#AppVersion}
english.HeroStatusSame=Version %1 installed · Reinstall or modify components
english.BtnInstallNow=Install Now
english.BtnUpgradeNow=Upgrade Now
english.BtnReinstall=Reinstall
english.BtnStartInstall=Install
english.BtnStartUpgrade=Upgrade
english.BtnClose=Close
english.BtnBrowse=Browse…
english.LinkCustomize=Customize ▸
english.LinkBack=← Back
english.LabelLocation=Location: %1
english.LabelCustomTitle=Custom Installation
english.LabelInstallDir=Install folder
english.ChooseDirPrompt=Choose a folder to install PureUtils
english.PrepTitle=Preparing installation…
english.PrepChecking=Checking running instances…
english.PrepClosing=Closing running %1…
english.ProgressInstalling=Installing PureUtils…
english.ProgressUpdating=Updating PureUtils…
english.FinishHeading=Installation Complete
english.FinishBody=PureUtils {#AppVersion} is ready to use.
english.TypeFull=Full (All Components)
english.TypeShotClip=Screenshot & Clipboard (PureShot + PureClip)
english.TypeViewer=PureImage Only
english.TypeVideo=PureVideo Only
english.TypeZip=PureZip Only
english.TypeCustom=Custom
english.CompCore=Core Runtime (Required)
english.CompCapture=PureShot — Screenshot & Pins
english.CompHistory=PureClip — Clipboard History
english.CompViewer=PureImage — Image Viewer & Space Preview
english.CompTurbo=JPEG Acceleration (libjpeg-turbo)
english.CompAssoc=Associate Image Formats
english.CompVideo=PureVideo — Video Player
english.CompVideoAssoc=Associate Video Formats
english.CompZip=PureZip — Archive Manager
english.CompZipContext=Explorer Context Menu
english.CompZipAssoc=Associate Archive Formats
english.TaskDesktopIcon=Create desktop shortcut
english.TaskAutostart=Run at sign-in
english.LaunchMain=Launch PureUtils
english.LaunchViewer=Launch PureImage
english.LaunchVideo=Launch PureVideo
english.LaunchZip=Launch PureZip
english.NeedModule=Please select at least one component.

[Types]
Name: "full"; Description: "{cm:TypeFull}"
Name: "shotclip"; Description: "{cm:TypeShotClip}"
Name: "viewer"; Description: "{cm:TypeViewer}"
Name: "video"; Description: "{cm:TypeVideo}"
Name: "zip"; Description: "{cm:TypeZip}"
Name: "custom"; Description: "{cm:TypeCustom}"; Flags: iscustom

[Components]
Name: "core"; Description: "{cm:CompCore}"; Types: full shotclip viewer video zip custom; Flags: fixed
Name: "capture"; Description: "{cm:CompCapture}"; Types: full shotclip custom
Name: "history"; Description: "{cm:CompHistory}"; Types: full shotclip custom
Name: "viewer"; Description: "{cm:CompViewer}"; Types: full viewer custom
Name: "viewer\turbo"; Description: "{cm:CompTurbo}"; Types: full viewer custom
Name: "viewer\assoc"; Description: "{cm:CompAssoc}"; Types: full viewer custom
Name: "video"; Description: "{cm:CompVideo}"; Types: full video custom
Name: "video\assoc"; Description: "{cm:CompVideoAssoc}"; Types: full video custom
Name: "zip"; Description: "{cm:CompZip}"; Types: full zip custom
Name: "zip\contextmenu"; Description: "{cm:CompZipContext}"; Types: full zip custom
Name: "zip\assoc"; Description: "{cm:CompZipAssoc}"; Types: full zip custom

[Tasks]
Name: "desktopicon"; Description: "{cm:TaskDesktopIcon}"; Flags: unchecked
Name: "autostart"; Description: "{cm:TaskAutostart}"; Flags: unchecked

[InstallDelete]
Type: files; Name: "{app}\turbojpeg.dll"
Type: files; Name: "{app}\libmpv-2.dll"
Type: files; Name: "{app}\resources\freeimage.ico"
Type: files; Name: "{app}\resources\image.ico"
Type: files; Name: "{app}\resources\image.png"
Type: files; Name: "{app}\resources\video.ico"
Type: files; Name: "{app}\resources\video.png"
Type: files; Name: "{app}\resources\zip.ico"
Type: files; Name: "{app}\resources\zip.png"
Type: files; Name: "{app}\ShotClip.exe"
Type: files; Name: "{app}\ynm3000.exe"

[Files]
Source: "{#StageDir}\resources\icon.png"; Flags: dontcopy
Source: "{#StageDir}\resources\btn-primary.png"; Flags: dontcopy
Source: "{#StageDir}\PureUtils-full.exe"; DestDir: "{app}"; DestName: "PureUtils.exe"; Components: viewer; Flags: ignoreversion
Source: "{#StageDir}\PureUtils-core.exe"; DestDir: "{app}"; DestName: "PureUtils.exe"; Components: not viewer; Flags: ignoreversion
Source: "{#StageDir}\turbojpeg.dll"; DestDir: "{app}"; Components: viewer\turbo; Flags: ignoreversion skipifsourcedoesntexist
Source: "{#StageDir}\libmpv-2.dll"; DestDir: "{app}"; Components: video; Flags: ignoreversion skipifsourcedoesntexist
Source: "{#StageDir}\resources\icon.png"; DestDir: "{app}\resources"; Flags: ignoreversion
Source: "{#StageDir}\resources\icon.ico"; DestDir: "{app}\resources"; Flags: ignoreversion
Source: "{#StageDir}\resources\image.ico"; DestDir: "{app}\resources"; Components: viewer; Flags: ignoreversion
Source: "{#StageDir}\resources\image.png"; DestDir: "{app}\resources"; Components: viewer; Flags: ignoreversion
Source: "{#StageDir}\resources\video.ico"; DestDir: "{app}\resources"; Components: video; Flags: ignoreversion
Source: "{#StageDir}\resources\video.png"; DestDir: "{app}\resources"; Components: video; Flags: ignoreversion
Source: "{#StageDir}\resources\zip.ico"; DestDir: "{app}\resources"; Components: zip; Flags: ignoreversion
Source: "{#StageDir}\resources\zip.png"; DestDir: "{app}\resources"; Components: zip; Flags: ignoreversion

[Icons]
Name: "{group}\PureUtils"; Filename: "{app}\PureUtils.exe"; IconFilename: "{app}\resources\icon.ico"; Components: capture or history
Name: "{group}\PureImage"; Filename: "{app}\PureUtils.exe"; Parameters: "--view"; IconFilename: "{app}\resources\image.ico"; AppUserModelID: "PureUtils.PureImage"; Components: viewer
Name: "{group}\PureVideo"; Filename: "{app}\PureUtils.exe"; Parameters: "--video"; IconFilename: "{app}\resources\video.ico"; AppUserModelID: "PureUtils.PureVideo"; Components: video
Name: "{group}\PureZip"; Filename: "{app}\PureUtils.exe"; Parameters: "--zip"; IconFilename: "{app}\resources\zip.ico"; AppUserModelID: "PureUtils.PureZip"; Components: zip
Name: "{autodesktop}\PureUtils"; Filename: "{app}\PureUtils.exe"; IconFilename: "{app}\resources\icon.ico"; Tasks: desktopicon; Components: capture or history
Name: "{autodesktop}\PureImage"; Filename: "{app}\PureUtils.exe"; Parameters: "--view"; IconFilename: "{app}\resources\image.ico"; AppUserModelID: "PureUtils.PureImage"; Tasks: desktopicon; Components: viewer and (not capture) and (not history)
Name: "{autodesktop}\PureVideo"; Filename: "{app}\PureUtils.exe"; Parameters: "--video"; IconFilename: "{app}\resources\video.ico"; AppUserModelID: "PureUtils.PureVideo"; Tasks: desktopicon; Components: video and (not capture) and (not history) and (not viewer)
Name: "{autodesktop}\PureZip"; Filename: "{app}\PureUtils.exe"; Parameters: "--zip"; IconFilename: "{app}\resources\zip.ico"; AppUserModelID: "PureUtils.PureZip"; Tasks: desktopicon; Components: zip and (not capture) and (not history) and (not viewer) and (not video)

[Registry]
; Optional autostart (standalone PureImage/PureVideo/PureZip installs launch to background tray without popping an empty window)
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "PureUtils"; ValueData: """{app}\PureUtils.exe"""; Flags: uninsdeletevalue; Tasks: autostart; Components: capture or history
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "PureUtils"; ValueData: """{app}\PureUtils.exe"" --background"; Flags: uninsdeletevalue; Tasks: autostart; Components: (viewer or video or zip) and (not capture) and (not history)

; Image file associations (HKCU) when viewer\assoc is checked
Root: HKCU; Subkey: "Software\Classes\PureUtils.Image"; ValueType: string; ValueName: ""; ValueData: "PureImage"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Image"; ValueType: string; ValueName: "FriendlyTypeName"; ValueData: "PureImage"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Image\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\resources\image.ico,0"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Image\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\PureUtils.exe"" --view ""%1"""; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Image\Application"; ValueType: string; ValueName: "ApplicationName"; ValueData: "PureImage"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Image\Application"; ValueType: string; ValueName: "ApplicationIcon"; ValueData: "{app}\resources\image.ico,0"; Flags: uninsdeletekey; Components: viewer\assoc

Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe"; ValueType: string; ValueName: "FriendlyAppName"; ValueData: "PureUtils"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\resources\image.ico,0"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\PureUtils.exe"" --view ""%1"""; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".jpg"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".jpeg"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".jpe"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".jfif"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".png"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".gif"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".webp"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".bmp"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".tif"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".tiff"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".heic"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".heif"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".hif"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".avif"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".avifs"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".jxl"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".cr2"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".cr3"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".crw"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".nef"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".nrw"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".arw"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".srf"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".sr2"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".orf"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".raf"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".rw2"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".dng"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".pef"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".srw"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".x3f"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".3fr"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".fff"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".iiq"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".mos"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".mrw"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".raw"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".rwl"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".kdc"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".dcr"; Flags: uninsdeletekey; Components: viewer\assoc

Root: HKCU; Subkey: "Software\RegisteredApplications"; ValueType: string; ValueName: "PureImage"; ValueData: "Software\PureUtils\PureImage\Capabilities"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities"; ValueType: string; ValueName: "ApplicationName"; ValueData: "PureImage"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities"; ValueType: string; ValueName: "ApplicationIcon"; ValueData: "{app}\resources\image.ico,0"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities"; ValueType: string; ValueName: "ApplicationDescription"; ValueData: "PureImage 高性能看图"; Flags: uninsdeletekey; Components: viewer\assoc

Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".jpg"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".jpeg"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".jpe"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".jfif"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".png"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".gif"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".webp"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".bmp"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".tif"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".tiff"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".heic"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".heif"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".hif"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".avif"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".avifs"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".jxl"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".cr2"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".cr3"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".crw"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".nef"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".nrw"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".arw"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".srf"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".sr2"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".orf"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".raf"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".rw2"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".dng"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".pef"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".srw"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".x3f"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".3fr"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".fff"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".iiq"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".mos"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".mrw"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".raw"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".rwl"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".kdc"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureImage\Capabilities\FileAssociations"; ValueType: string; ValueName: ".dcr"; ValueData: "PureUtils.Image"; Flags: uninsdeletekey; Components: viewer\assoc

Root: HKCU; Subkey: "Software\Classes\.jpg\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.jpeg\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.jpe\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.jfif\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.png\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.gif\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.webp\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.bmp\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.tif\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.tiff\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.heic\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.heif\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.hif\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.avif\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.avifs\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.jxl\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.cr2\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.cr3\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.crw\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.nef\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.nrw\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.arw\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.srf\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.sr2\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.orf\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.raf\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.rw2\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.dng\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.pef\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.srw\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.x3f\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.3fr\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.fff\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.iiq\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.mos\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.mrw\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.raw\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.rwl\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.kdc\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc
Root: HKCU; Subkey: "Software\Classes\.dcr\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Image"; Flags: uninsdeletevalue; Components: viewer\assoc

; Archive file associations (HKCU) when zip\assoc is checked
Root: HKCU; Subkey: "Software\Classes\PureUtils.Zip"; ValueType: string; ValueName: ""; ValueData: "压缩文件"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Zip"; ValueType: string; ValueName: "FriendlyTypeName"; ValueData: "压缩文件 (PureZip)"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Zip\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\resources\zip.ico,0"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Zip\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\PureUtils.exe"" --zip ""%1"""; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Zip\Application"; ValueType: string; ValueName: "ApplicationName"; ValueData: "PureZip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Zip\Application"; ValueType: string; ValueName: "ApplicationIcon"; ValueData: "{app}\resources\zip.ico,0"; Flags: uninsdeletekey; Components: zip\assoc

Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe"; ValueType: string; ValueName: "FriendlyAppName"; ValueData: "PureUtils"; Flags: uninsdeletekey; Components: zip\assoc and (not viewer\assoc)
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\PureUtils.exe"" ""%1"""; Flags: uninsdeletekey; Components: zip\assoc and (not viewer\assoc)
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".zip"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".7z"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".rar"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".tar"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".gz"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".tgz"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".bz2"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".tbz"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".tbz2"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".xz"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".txz"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".zst"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".tzst"; Flags: uninsdeletevalue; Components: zip\assoc

Root: HKCU; Subkey: "Software\RegisteredApplications"; ValueType: string; ValueName: "PureZip"; ValueData: "Software\PureUtils\PureZip\Capabilities"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities"; ValueType: string; ValueName: "ApplicationName"; ValueData: "PureZip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities"; ValueType: string; ValueName: "ApplicationIcon"; ValueData: "{app}\resources\zip.ico,0"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities"; ValueType: string; ValueName: "ApplicationDescription"; ValueData: "PureZip 压缩管理"; Flags: uninsdeletekey; Components: zip\assoc

Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: ".zip"; ValueData: "PureUtils.Zip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: ".7z"; ValueData: "PureUtils.Zip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: ".rar"; ValueData: "PureUtils.Zip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: ".tar"; ValueData: "PureUtils.Zip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: ".gz"; ValueData: "PureUtils.Zip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: ".tgz"; ValueData: "PureUtils.Zip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: ".bz2"; ValueData: "PureUtils.Zip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: ".tbz"; ValueData: "PureUtils.Zip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: ".tbz2"; ValueData: "PureUtils.Zip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: ".xz"; ValueData: "PureUtils.Zip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: ".txz"; ValueData: "PureUtils.Zip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: ".zst"; ValueData: "PureUtils.Zip"; Flags: uninsdeletekey; Components: zip\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureZip\Capabilities\FileAssociations"; ValueType: string; ValueName: ".tzst"; ValueData: "PureUtils.Zip"; Flags: uninsdeletekey; Components: zip\assoc

Root: HKCU; Subkey: "Software\Classes\.zip\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\.7z\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\.rar\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\.tar\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\.gz\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\.tgz\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\.bz2\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\.tbz\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\.tbz2\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\.xz\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\.txz\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\.zst\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"; Flags: uninsdeletevalue; Components: zip\assoc
Root: HKCU; Subkey: "Software\Classes\.tzst\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Zip"; Flags: uninsdeletevalue; Components: zip\assoc

; Video file associations (HKCU) when video\assoc is checked
Root: HKCU; Subkey: "Software\Classes\PureUtils.Video"; ValueType: string; ValueName: ""; ValueData: "PureVideo"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Video"; ValueType: string; ValueName: "FriendlyTypeName"; ValueData: "PureVideo"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Video\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\resources\video.ico,0"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Video\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\PureUtils.exe"" --video ""%1"""; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Video\Application"; ValueType: string; ValueName: "ApplicationName"; ValueData: "PureVideo"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\PureUtils.Video\Application"; ValueType: string; ValueName: "ApplicationIcon"; ValueData: "{app}\resources\video.ico,0"; Flags: uninsdeletekey; Components: video\assoc

Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe"; ValueType: string; ValueName: "FriendlyAppName"; ValueData: "PureUtils"; Flags: uninsdeletekey; Components: video\assoc and (not viewer\assoc) and (not zip\assoc)
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\resources\video.ico,0"; Flags: uninsdeletekey; Components: video\assoc and (not viewer\assoc) and (not zip\assoc)
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\PureUtils.exe"" --video ""%1"""; Flags: uninsdeletekey; Components: video\assoc and (not viewer\assoc) and (not zip\assoc)
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".mp4"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".m4v"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".mov"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".mkv"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".webm"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".avi"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".wmv"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".ts"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".m2ts"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".mts"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".mpg"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".mpeg"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".3gp"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".3g2"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".asf"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".flv"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".rmvb"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".rm"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".ogv"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".vob"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\Applications\PureUtils.exe\SupportedTypes"; ValueType: none; ValueName: ".f4v"; Flags: uninsdeletevalue; Components: video\assoc

Root: HKCU; Subkey: "Software\RegisteredApplications"; ValueType: string; ValueName: "PureVideo"; ValueData: "Software\PureUtils\PureVideo\Capabilities"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities"; ValueType: string; ValueName: "ApplicationName"; ValueData: "PureVideo"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities"; ValueType: string; ValueName: "ApplicationIcon"; ValueData: "{app}\resources\video.ico,0"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities"; ValueType: string; ValueName: "ApplicationDescription"; ValueData: "PureVideo 极速视频浏览"; Flags: uninsdeletekey; Components: video\assoc

Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".mp4"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".m4v"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".mov"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".mkv"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".webm"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".avi"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".wmv"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".ts"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".m2ts"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".mts"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".mpg"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".mpeg"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".3gp"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".3g2"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".asf"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".flv"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".rmvb"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".rm"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".ogv"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".vob"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc
Root: HKCU; Subkey: "Software\PureUtils\PureVideo\Capabilities\FileAssociations"; ValueType: string; ValueName: ".f4v"; ValueData: "PureUtils.Video"; Flags: uninsdeletekey; Components: video\assoc

Root: HKCU; Subkey: "Software\Classes\.mp4\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.m4v\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.mov\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.mkv\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.webm\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.avi\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.wmv\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.ts\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.m2ts\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.mts\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.mpg\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.mpeg\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.3gp\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.3g2\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.asf\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.flv\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.rmvb\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.rm\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.ogv\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.vob\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc
Root: HKCU; Subkey: "Software\Classes\.f4v\OpenWithProgids"; ValueType: none; ValueName: "PureUtils.Video"; Flags: uninsdeletevalue; Components: video\assoc

[Run]
Filename: "{app}\PureUtils.exe"; Parameters: ""; Description: "{cm:LaunchMain}"; Flags: nowait postinstall skipifsilent; Components: capture or history
Filename: "{app}\PureUtils.exe"; Parameters: "--view"; Description: "{cm:LaunchViewer}"; Flags: nowait postinstall skipifsilent; Components: viewer and (not capture) and (not history)
Filename: "{app}\PureUtils.exe"; Parameters: "--video"; Description: "{cm:LaunchVideo}"; Flags: nowait postinstall skipifsilent; Components: video and (not capture) and (not history) and (not viewer)
Filename: "{app}\PureUtils.exe"; Parameters: "--zip"; Description: "{cm:LaunchZip}"; Flags: nowait postinstall skipifsilent; Components: zip and (not capture) and (not history) and (not viewer) and (not video)
Filename: "{app}\PureUtils.exe"; Parameters: "--background"; Flags: nowait skipifnotsilent; Components: capture or history
Filename: "{app}\PureUtils.exe"; Parameters: "--settings"; Flags: nowait skipifnotsilent; Components: (not capture) and (not history)

[UninstallRun]
Filename: "{app}\PureUtils.exe"; Parameters: "--exit"; Flags: runhidden; RunOnceId: "ExitPureUtils"
Filename: "{cmd}"; Parameters: "/C taskkill /IM PureUtils.exe /F"; Flags: runhidden; RunOnceId: "KillPureUtils"

[UninstallDelete]
Type: filesandordirs; Name: "{app}"

[Code]
#if VER >= 0x07000000
procedure SHChangeNotify(wEventId: LongInt; uFlags: Cardinal; dwItem1: NativeInt; dwItem2: NativeInt);
  external 'SHChangeNotify@shell32.dll stdcall';
function DwmSetWindowAttribute(hWnd: HWND; dwAttribute: DWORD; var pvAttribute: DWORD; cbAttribute: DWORD): LongInt;
  external 'DwmSetWindowAttribute@dwmapi.dll stdcall delayload';
#else
procedure SHChangeNotify(wEventId: LongInt; uFlags: Cardinal; dwItem1: LongInt; dwItem2: LongInt);
  external 'SHChangeNotify@shell32.dll stdcall';
function DwmSetWindowAttribute(hWnd: LongInt; dwAttribute: Cardinal; var pvAttribute: Cardinal; cbAttribute: Cardinal): LongInt;
  external 'DwmSetWindowAttribute@dwmapi.dll stdcall delayload';
#endif

function BoolToJson(B: Boolean): String;
begin
  if B then Result := 'true' else Result := 'false';
end;

function ImageExtensions: String;
begin
  Result := 'jpg,jpeg,jpe,jfif,png,gif,webp,bmp,tif,tiff,heic,heif,hif,avif,avifs,jxl,cr2,cr3,crw,nef,nrw,arw,srf,sr2,orf,raf,rw2,dng,pef,srw,x3f,3fr,fff,iiq,mos,mrw,raw,rwl,kdc,dcr';
end;

function ZipExtensions: String;
begin
  Result := 'zip,7z,rar,tar,gz,tgz,bz2,tbz,tbz2,xz,txz,zst,tzst';
end;

function VideoExtensions: String;
begin
  Result := 'mp4,m4v,mov,mkv,webm,avi,wmv,ts,m2ts,mts,mpg,mpeg,3gp,3g2,asf,flv,rmvb,rm,ogv,vob,f4v';
end;

procedure CleanupExeSupportedTypes(ExeKey, CommaExts: String);
var
  Exts: TStringList;
  Left: TArrayOfString;
  I: Integer;
begin
  Exts := TStringList.Create;
  try
    Exts.CommaText := CommaExts;
    for I := 0 to Exts.Count - 1 do
      RegDeleteValue(HKCU, 'Software\Classes\Applications\' + ExeKey + '\SupportedTypes', '.' + Exts[I]);
  finally
    Exts.Free;
  end;
  if not RegGetValueNames(HKCU, 'Software\Classes\Applications\' + ExeKey + '\SupportedTypes', Left) or (GetArrayLength(Left) = 0) then
    RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\Applications\' + ExeKey);
end;

procedure CleanupLegacyAssociations;
var
  Exts: TStringList;
  I: Integer;
begin
  // Legacy ShotClip cleanup
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\ShotClip.FreeImage');
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\ShotClip\FreeImage');
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\Applications\ShotClip.exe');
  RegDeleteValue(HKCU, 'Software\RegisteredApplications', 'ShotClip FreeImage');
  RegDeleteValue(HKCU, 'Software\Microsoft\Windows\CurrentVersion\Run', 'ShotClip');

  // Legacy ynm3000 cleanup
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\ynm3000.Image');
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\ynm3000\Image');
  RegDeleteValue(HKCU, 'Software\RegisteredApplications', 'ynm3000 Image');
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\ynm3000.Zip');
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\ynm3000\Zip');
  RegDeleteValue(HKCU, 'Software\RegisteredApplications', 'ynm3000 Zip');
  RegDeleteValue(HKCU, 'Software\RegisteredApplications', 'Zip');
  RegDeleteValue(HKCU, 'Software\Microsoft\Windows\CurrentVersion\Run', 'ynm3000');
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\Applications\ynm3000.exe');
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\*\shell\ynm3000.Zip');
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\Directory\shell\ynm3000.Zip');

  Exts := TStringList.Create;
  try
    Exts.CommaText := ImageExtensions;
    for I := 0 to Exts.Count - 1 do
    begin
      RegDeleteValue(HKCU, 'Software\Classes\.' + Exts[I] + '\OpenWithProgids', 'ShotClip.FreeImage');
      RegDeleteValue(HKCU, 'Software\Classes\.' + Exts[I] + '\OpenWithProgids', 'ynm3000.Image');
    end;
    Exts.CommaText := ZipExtensions;
    for I := 0 to Exts.Count - 1 do
    begin
      RegDeleteValue(HKCU, 'Software\Classes\.' + Exts[I] + '\OpenWithProgids', 'ynm3000.Zip');
      RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\SystemFileAssociations\.' + Exts[I] + '\shell\ynm3000.ZipArchive');
    end;
  finally
    Exts.Free;
  end;
end;

procedure CleanupOldAssociations;
var
  Exts: TStringList;
  I: Integer;
begin
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\PureUtils.Image');
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\PureUtils\PureImage');
  RegDeleteValue(HKCU, 'Software\RegisteredApplications', 'PureImage');
  Exts := TStringList.Create;
  try
    Exts.CommaText := ImageExtensions;
    for I := 0 to Exts.Count - 1 do
      RegDeleteValue(HKCU, 'Software\Classes\.' + Exts[I] + '\OpenWithProgids', 'PureUtils.Image');
  finally
    Exts.Free;
  end;
  CleanupExeSupportedTypes('PureUtils.exe', ImageExtensions);
end;

procedure CleanupOldZipAssociations;
var
  Exts: TStringList;
  I: Integer;
begin
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\PureUtils.Zip');
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\PureUtils\PureZip');
  RegDeleteValue(HKCU, 'Software\RegisteredApplications', 'PureZip');
  Exts := TStringList.Create;
  try
    Exts.CommaText := ZipExtensions;
    for I := 0 to Exts.Count - 1 do
      RegDeleteValue(HKCU, 'Software\Classes\.' + Exts[I] + '\OpenWithProgids', 'PureUtils.Zip');
  finally
    Exts.Free;
  end;
  CleanupExeSupportedTypes('PureUtils.exe', ZipExtensions);
end;

procedure CleanupOldVideoAssociations;
var
  Exts: TStringList;
  I: Integer;
begin
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\PureUtils.Video');
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\PureUtils\PureVideo');
  RegDeleteValue(HKCU, 'Software\RegisteredApplications', 'PureVideo');
  Exts := TStringList.Create;
  try
    Exts.CommaText := VideoExtensions;
    for I := 0 to Exts.Count - 1 do
      RegDeleteValue(HKCU, 'Software\Classes\.' + Exts[I] + '\OpenWithProgids', 'PureUtils.Video');
  finally
    Exts.Free;
  end;
  CleanupExeSupportedTypes('PureUtils.exe', VideoExtensions);
end;

procedure WriteZipVerb(BaseKey, VerbID, LabelText, ExePath, Arg: String);
var
  VerbKey: String;
begin
  VerbKey := BaseKey + '\shell\' + VerbID;
  RegWriteStringValue(HKCU, VerbKey, 'MUIVerb', LabelText);
  RegWriteStringValue(HKCU, VerbKey, 'MultiSelectModel', 'Player');
  RegWriteStringValue(HKCU, VerbKey + '\command', '', '"' + ExePath + '" ' + Arg + ' "%1"');
end;

procedure WriteZipCascadeHeader(BaseKey, IconVal, AppliesTo: String);
begin
  RegWriteStringValue(HKCU, BaseKey, 'MUIVerb', 'PureZip');
  RegWriteStringValue(HKCU, BaseKey, 'Icon', IconVal);
  RegWriteStringValue(HKCU, BaseKey, 'SubCommands', '');
  RegWriteStringValue(HKCU, BaseKey, 'MultiSelectModel', 'Player');
  if AppliesTo <> '' then
    RegWriteStringValue(HKCU, BaseKey, 'AppliesTo', AppliesTo);
end;

procedure RegisterZipContextMenu;
var
  ExePath, IconVal, AppliesNotArchive, BaseKey: String;
  Exts: TStringList;
  I: Integer;
begin
  ExePath := ExpandConstant('{app}\PureUtils.exe');
  IconVal := ExpandConstant('{app}\resources\zip.ico,0');
  Exts := TStringList.Create;
  try
    Exts.CommaText := ZipExtensions;
    AppliesNotArchive := '';
    for I := 0 to Exts.Count - 1 do
    begin
      if I > 0 then
        AppliesNotArchive := AppliesNotArchive + ' OR ';
      AppliesNotArchive := AppliesNotArchive + 'System.FileExtension:=.' + Exts[I];
    end;
    AppliesNotArchive := 'NOT (' + AppliesNotArchive + ')';

    BaseKey := 'Software\Classes\*\shell\PureUtils.PureZip';
    WriteZipCascadeHeader(BaseKey, IconVal, AppliesNotArchive);
    WriteZipVerb(BaseKey, '10compress', '添加到压缩文件…', ExePath, '--compress');
    WriteZipVerb(BaseKey, '11zip', '压缩为同名 .zip', ExePath, '--compress-zip');
    WriteZipVerb(BaseKey, '12sevenzip', '压缩为同名 .7z', ExePath, '--compress-7z');

    BaseKey := 'Software\Classes\Directory\shell\PureUtils.PureZip';
    WriteZipCascadeHeader(BaseKey, IconVal, '');
    WriteZipVerb(BaseKey, '10compress', '添加到压缩文件…', ExePath, '--compress');
    WriteZipVerb(BaseKey, '11zip', '压缩为同名 .zip', ExePath, '--compress-zip');
    WriteZipVerb(BaseKey, '12sevenzip', '压缩为同名 .7z', ExePath, '--compress-7z');

    for I := 0 to Exts.Count - 1 do
    begin
      BaseKey := 'Software\Classes\SystemFileAssociations\.' + Exts[I] + '\shell\PureUtils.PureZipArchive';
      WriteZipCascadeHeader(BaseKey, IconVal, '');
      WriteZipVerb(BaseKey, '01smart', '智能解压（自动判断是否新建文件夹）', ExePath, '--extract-smart');
      WriteZipVerb(BaseKey, '02here', '解压到当前文件夹', ExePath, '--extract-here');
      WriteZipVerb(BaseKey, '03folder', '解压到同名文件夹', ExePath, '--extract-to');
      WriteZipVerb(BaseKey, '04dialog', '解压到…', ExePath, '--extract-dialog');
      WriteZipVerb(BaseKey, '05open', '用 PureZip 打开', ExePath, '--zip');
      WriteZipVerb(BaseKey, '06test', '测试压缩文件', ExePath, '--test-archive');
      WriteZipVerb(BaseKey, '10compress', '添加到压缩文件…', ExePath, '--compress');
    end;
  finally
    Exts.Free;
  end;
end;

procedure UnregisterZipContextMenu;
var
  Exts: TStringList;
  I: Integer;
begin
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\*\shell\PureUtils.PureZip');
  RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\Directory\shell\PureUtils.PureZip');
  Exts := TStringList.Create;
  try
    Exts.CommaText := ZipExtensions;
    for I := 0 to Exts.Count - 1 do
      RegDeleteKeyIncludingSubkeys(HKCU, 'Software\Classes\SystemFileAssociations\.' + Exts[I] + '\shell\PureUtils.PureZipArchive');
  finally
    Exts.Free;
  end;
end;

var
  CustomMode, NavPending: Boolean;
  InstalledOldVersion: String;
  InstallKind: Integer;
  HeroPathLabel, FinishSubLabel, PrepTitleLabel, PrepStatusLabel, ProgTitleLabel: TNewStaticText;
  HeroPrimaryLabel, CustomPrimaryLabel, FinishPrimaryLabel: TLabel;
  HeroCustomizeLink, CustomBackLink, FinishCloseLink: TLabel;
  PrepLogo: TBitmapImage;
  CustomDirEdit: TNewEdit;
  CustomBrowseBtn: TNewButton;
  CustomDesktopCheck, CustomAutostartCheck: TNewCheckBox;

function IsProcessRunning(const ExeName: String; var QueryOK: Boolean): Boolean;
var
  WMILocator, WMIService, WMIResult: Variant;
begin
  QueryOK := False;
  Result := False;
  try
    WMILocator := CreateOleObject('WbemScripting.SWbemLocator');
    WMIService := WMILocator.ConnectServer('.', 'root\cimv2');
    WMIResult := WMIService.ExecQuery('SELECT ProcessId FROM Win32_Process WHERE Name=''' + ExeName + '''');
    QueryOK := True;
    Result := WMIResult.Count > 0;
  except
    QueryOK := False;
    Result := False;
  end;
end;

procedure StopRunningApp(const ExePath, ProcName, DisplayName: String);
var
  QueryOK, Running: Boolean;
  ResultCode, I: Integer;
begin
  if not FileExists(ExePath) then Exit;
  Running := IsProcessRunning(ProcName, QueryOK);
  if QueryOK then
  begin
    if not Running then Exit;
    if PrepStatusLabel <> nil then
    begin
      PrepStatusLabel.Caption := FmtMessage(CustomMessage('PrepClosing'), [DisplayName]);
      WizardForm.Update;
    end;
    Exec(ExePath, '--exit', '', SW_HIDE, ewNoWait, ResultCode);
    for I := 1 to 10 do
    begin
      Sleep(100);
      WizardForm.Update;
      Running := IsProcessRunning(ProcName, QueryOK);
      if QueryOK and (not Running) then Exit;
    end;
    Exec(ExpandConstant('{sys}\taskkill.exe'), '/IM ' + ProcName + ' /F', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
    for I := 1 to 5 do
    begin
      Sleep(100);
      WizardForm.Update;
      Running := IsProcessRunning(ProcName, QueryOK);
      if QueryOK and (not Running) then Exit;
    end;
  end else begin
    Exec(ExpandConstant('{sys}\taskkill.exe'), '/IM ' + ProcName + ' /F', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
    Sleep(200);
  end;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := '';
  WizardForm.Update;
  StopRunningApp(ExpandConstant('{app}\PureUtils.exe'), 'PureUtils.exe', 'PureUtils');
  StopRunningApp(ExpandConstant('{localappdata}\Programs\ynm3000\ynm3000.exe'), 'ynm3000.exe', 'ynm3000');
  StopRunningApp(ExpandConstant('{localappdata}\Programs\ShotClip\ShotClip.exe'), 'ShotClip.exe', 'ShotClip');
end;

function InkPrimary: TColor;
begin
  if IsDarkInstallMode then
    Result := StrToColor('#F4F4F5')
  else
    Result := StrToColor('#141413');
end;

function InkSecondary: TColor;
begin
  if IsDarkInstallMode then
    Result := StrToColor('#A1A1AA')
  else
    Result := StrToColor('#57534E');
end;

function InkMuted: TColor;
begin
  if IsDarkInstallMode then
    Result := StrToColor('#71717A')
  else
    Result := StrToColor('#78716C');
end;

function InkAccent: TColor;
begin
  if IsDarkInstallMode then
    Result := StrToColor('#818CF8')
  else
    Result := StrToColor('#4F46E5');
end;

procedure ApplySeamlessTitleBar(
#if VER >= 0x07000000
  Wnd: HWND
#else
  Wnd: LongInt
#endif
);
var
#if VER >= 0x07000000
  DarkVal, CaptionColor: DWORD;
#else
  DarkVal, CaptionColor: Cardinal;
#endif
begin
  if Wnd = 0 then Exit;
  try
    if IsDarkInstallMode then
    begin
      DarkVal := 1;
      CaptionColor := $001B1818;
    end else begin
      DarkVal := 0;
      CaptionColor := $00FCFAF8;
    end;
    DwmSetWindowAttribute(Wnd, 20, DarkVal, SizeOf(DarkVal));
    DwmSetWindowAttribute(Wnd, 35, CaptionColor, SizeOf(CaptionColor));
  except
  end;
end;

procedure DetectInstalledVersion;
var
  UninstKey: String;
begin
  InstalledOldVersion := '';
  InstallKind := 0;
  UninstKey := 'Software\Microsoft\Windows\CurrentVersion\Uninstall\{6D9E4A1B-3C82-4F55-9A14-2B7E8C3D5F10}_is1';
  if RegQueryStringValue(HKCU, UninstKey, 'DisplayVersion', InstalledOldVersion) and (Trim(InstalledOldVersion) <> '') then
  begin
    InstalledOldVersion := Trim(InstalledOldVersion);
    if CompareText(InstalledOldVersion, '{#AppVersion}') = 0 then
      InstallKind := 2
    else
      InstallKind := 1;
  end;
end;

function ShortenDisplayPath(P: String; MaxLen: Integer): String;
begin
  if Length(P) <= MaxLen then
    Result := P
  else
    Result := Copy(P, 1, 14) + '…' + Copy(P, Length(P) - (MaxLen - 16), MaxLen - 15);
end;

procedure RefreshHeroPath;
var
  DirPath: String;
begin
  if HeroPathLabel = nil then Exit;
  DirPath := RemoveBackslashUnlessRoot(Trim(WizardForm.DirEdit.Text));
  HeroPathLabel.Caption := FmtMessage(CustomMessage('LabelLocation'), [ShortenDisplayPath(DirPath, 38)]);
  HeroPathLabel.Hint := DirPath;
end;

procedure SyncTasksToUI;
begin
  if (CustomDesktopCheck <> nil) and (WizardForm.TasksList.Items.Count >= 1) then
    CustomDesktopCheck.Checked := WizardForm.TasksList.Checked[0];
  if (CustomAutostartCheck <> nil) and (WizardForm.TasksList.Items.Count >= 2) then
    CustomAutostartCheck.Checked := WizardForm.TasksList.Checked[1];
end;

procedure SyncTasksFromUI;
begin
  if (CustomDesktopCheck <> nil) and (WizardForm.TasksList.Items.Count >= 1) then
    WizardForm.TasksList.Checked[0] := CustomDesktopCheck.Checked;
  if (CustomAutostartCheck <> nil) and (WizardForm.TasksList.Items.Count >= 2) then
    WizardForm.TasksList.Checked[1] := CustomAutostartCheck.Checked;
end;

procedure ResizeAndCenterWindow(LogicalW, LogicalH: Integer);
var
  NewClientW, NewClientH, FrameW, FrameH, NewLeft, NewTop: Integer;
begin
  NewClientW := ScaleX(LogicalW);
  NewClientH := ScaleY(LogicalH);
  if (WizardForm.ClientWidth <> NewClientW) or (WizardForm.ClientHeight <> NewClientH) then
  begin
    FrameW := WizardForm.Width - WizardForm.ClientWidth;
    FrameH := WizardForm.Height - WizardForm.ClientHeight;
    NewLeft := WizardForm.Left + (WizardForm.Width - (NewClientW + FrameW)) div 2;
    NewTop := WizardForm.Top + (WizardForm.Height - (NewClientH + FrameH)) div 2;
    if NewLeft < 0 then NewLeft := 0;
    if NewTop < 0 then NewTop := 0;
    WizardForm.SetBounds(NewLeft, NewTop, NewClientW + FrameW, NewClientH + FrameH);
  end;
  WizardForm.OuterNotebook.SetBounds(0, 0, WizardForm.ClientWidth, WizardForm.ClientHeight);
  WizardForm.InnerNotebook.SetBounds(0, 0, WizardForm.ClientWidth, WizardForm.ClientHeight);
end;

procedure ParkOffscreen(Ctrl: TWinControl);
begin
  Ctrl.TabStop := False;
  Ctrl.SetBounds(0, WizardForm.ClientHeight + ScaleY(200), ScaleX(80), ScaleY(24));
end;

function InitLogoImage(ParentCtrl: TWinControl; WinW, TopDp: Integer; IconFile: String): TBitmapImage;
var
  Sz: Integer;
begin
  Sz := ScaleX(64);
  Result := TBitmapImage.Create(WizardForm);
  Result.Parent := ParentCtrl;
  Result.SetBounds((ScaleX(WinW) - Sz) div 2, ScaleY(TopDp), Sz, Sz);
  Result.BackColor := clNone;
  Result.Stretch := True;
  Result.Center := True;
  Result.PngImage.LoadFromFile(IconFile);
end;

function CreatePrimaryBtnVisual(
  ParentCtrl: TWinControl;
  LeftDp, TopDp, WidthDp, HeightDp: Integer;
  BtnFile, CaptionText: String;
  var OutLbl: TLabel
): TBitmapImage;
var
  X, Y, W, H, TextH: Integer;
begin
  X := ScaleX(LeftDp);
  Y := ScaleY(TopDp);
  W := ScaleX(WidthDp);
  H := ScaleY(HeightDp);

  Result := TBitmapImage.Create(WizardForm);
  Result.Parent := ParentCtrl;
  Result.SetBounds(X, Y, W, H);
  Result.BackColor := clNone;
  Result.Stretch := True;
  Result.PngImage.LoadFromFile(BtnFile);
  Result.Cursor := crHand;

  OutLbl := TLabel.Create(WizardForm);
  OutLbl.Parent := ParentCtrl;
  OutLbl.Transparent := True;
  OutLbl.WordWrap := False;
  OutLbl.StyleElements := OutLbl.StyleElements - [seFont];
  OutLbl.Font.Name := WizardForm.Font.Name;
  OutLbl.Font.Size := 10;
  OutLbl.Font.Style := [fsBold];
  OutLbl.Font.Color := clWhite;
  OutLbl.AutoSize := True;
  OutLbl.Caption := CaptionText;
  TextH := OutLbl.Height;
  OutLbl.AutoSize := False;
  OutLbl.Alignment := taCenter;
  OutLbl.SetBounds(X, Y + (H - TextH) div 2, W, TextH + ScaleY(2));
  OutLbl.Cursor := crHand;
  OutLbl.BringToFront;
end;

procedure BrowseInstallDirOnClick(Sender: TObject);
var
  Dir: String;
begin
  Dir := Trim(WizardForm.DirEdit.Text);
  if (CustomMode) and (CustomDirEdit <> nil) and (Trim(CustomDirEdit.Text) <> '') then
    Dir := Trim(CustomDirEdit.Text);
  if BrowseForFolder(CustomMessage('ChooseDirPrompt'), Dir, True) then
  begin
    Dir := RemoveBackslashUnlessRoot(Dir);
    if CompareText(ExtractFileName(Dir), 'PureUtils') <> 0 then
      Dir := AddBackslash(Dir) + 'PureUtils';
    WizardForm.DirEdit.Text := Dir;
    if CustomDirEdit <> nil then
      CustomDirEdit.Text := Dir;
    RefreshHeroPath;
  end;
end;

procedure CustomTaskCheckOnClick(Sender: TObject);
begin
  SyncTasksFromUI;
end;

procedure TriggerWizardBtn(Btn: TNewButton);
begin
  if NavPending then Exit;
  NavPending := True;
  PostMessage(Btn.Handle, $BD11, 0, 0);
end;

procedure HeroPrimaryOnClick(Sender: TObject);
begin
  CustomMode := False;
  TriggerWizardBtn(WizardForm.NextButton);
end;

procedure HeroCustomizeOnClick(Sender: TObject);
begin
  CustomMode := True;
  TriggerWizardBtn(WizardForm.NextButton);
end;

procedure CustomPrimaryOnClick(Sender: TObject);
begin
  TriggerWizardBtn(WizardForm.NextButton);
end;

procedure CustomBackOnClick(Sender: TObject);
begin
  CustomMode := False;
  if CustomDirEdit <> nil then
    WizardForm.DirEdit.Text := RemoveBackslashUnlessRoot(Trim(CustomDirEdit.Text));
  SyncTasksFromUI;
  RefreshHeroPath;
  TriggerWizardBtn(WizardForm.BackButton);
end;

procedure FinishPrimaryOnClick(Sender: TObject);
begin
  TriggerWizardBtn(WizardForm.NextButton);
end;

procedure FinishCloseOnClick(Sender: TObject);
var
  I: Integer;
begin
  for I := 0 to WizardForm.RunList.Items.Count - 1 do
    WizardForm.RunList.Checked[I] := False;
  TriggerWizardBtn(WizardForm.NextButton);
end;

procedure InitializeWizard;
var
  IconPath, BtnPath, StatusText: String;
  Lbl, DirLbl: TNewStaticText;
  BtnImg, LogoImg: TBitmapImage;
  SpaceW, BrowseW: Integer;
begin
  CustomMode := False;
  NavPending := False;
  DetectInstalledVersion;
  ApplySeamlessTitleBar(WizardForm.Handle);

  ExtractTemporaryFile('icon.png');
  ExtractTemporaryFile('btn-primary.png');
  IconPath := ExpandConstant('{tmp}\icon.png');
  BtnPath := ExpandConstant('{tmp}\btn-primary.png');

  WizardForm.BorderIcons := [biSystemMenu];
  WizardForm.OuterNotebook.Anchors := [akLeft, akTop];
  WizardForm.InnerNotebook.Anchors := [akLeft, akTop];
  WizardForm.NextButton.Anchors := [akLeft, akTop];
  WizardForm.BackButton.Anchors := [akLeft, akTop];
  WizardForm.CancelButton.Anchors := [akLeft, akTop];
  WizardForm.TypesCombo.Anchors := [akLeft, akTop];
  WizardForm.ComponentsList.Anchors := [akLeft, akTop];
  WizardForm.ComponentsDiskSpaceLabel.Anchors := [akLeft, akTop];
  WizardForm.PreparingLabel.Anchors := [akLeft, akTop];
  WizardForm.PreparingMemo.Anchors := [akLeft, akTop];
  WizardForm.PreparingYesRadio.Anchors := [akLeft, akTop];
  WizardForm.PreparingNoRadio.Anchors := [akLeft, akTop];
  WizardForm.ProgressGauge.Anchors := [akLeft, akTop];
  WizardForm.StatusLabel.Anchors := [akLeft, akTop];
  WizardForm.RunList.Anchors := [akLeft, akTop];
  WizardForm.YesRadio.Anchors := [akLeft, akTop];
  WizardForm.NoRadio.Anchors := [akLeft, akTop];
  WizardForm.MainPanel.Visible := False;
  WizardForm.Bevel1.Visible := False;
  WizardForm.Bevel.Visible := False;
  WizardForm.BeveledLabel.Visible := False;

  ResizeAndCenterWindow(500, 370);
  WizardForm.OuterNotebook.SendToBack;

  // --- Page 1: Compact Hero Page (500 x 370) ---
  WizardForm.WizardBitmapImage.Visible := False;
  WizardForm.WelcomeLabel1.Visible := False;
  WizardForm.WelcomeLabel2.Visible := False;

  LogoImg := InitLogoImage(WizardForm.WelcomePage, 500, 34, IconPath);

  Lbl := TNewStaticText.Create(WizardForm);
  Lbl.Parent := WizardForm.WelcomePage;
  Lbl.AutoSize := False;
  Lbl.SetBounds(ScaleX(24), ScaleY(108), ScaleX(452), ScaleY(38));
  Lbl.Alignment := taCenter;
  Lbl.StyleElements := Lbl.StyleElements - [seFont];
  Lbl.Font.Name := WizardForm.Font.Name;
  Lbl.Font.Size := 20;
  Lbl.Font.Style := [fsBold];
  Lbl.Font.Color := InkPrimary;
  Lbl.Caption := 'PureUtils';

  Lbl := TNewStaticText.Create(WizardForm);
  Lbl.Parent := WizardForm.WelcomePage;
  Lbl.AutoSize := False;
  Lbl.SetBounds(ScaleX(24), ScaleY(152), ScaleX(452), ScaleY(24));
  Lbl.Alignment := taCenter;
  Lbl.StyleElements := Lbl.StyleElements - [seFont];
  Lbl.Font.Name := WizardForm.Font.Name;
  Lbl.Font.Size := 10;
  Lbl.Font.Color := InkSecondary;
  Lbl.Caption := CustomMessage('HeroTagline');

  if InstallKind = 1 then
    StatusText := FmtMessage(CustomMessage('HeroStatusUpgrade'), [InstalledOldVersion])
  else if InstallKind = 2 then
    StatusText := FmtMessage(CustomMessage('HeroStatusSame'), [InstalledOldVersion])
  else
    StatusText := CustomMessage('HeroStatusNew');

  Lbl := TNewStaticText.Create(WizardForm);
  Lbl.Parent := WizardForm.WelcomePage;
  Lbl.AutoSize := False;
  Lbl.SetBounds(ScaleX(24), ScaleY(182), ScaleX(452), ScaleY(20));
  Lbl.Alignment := taCenter;
  Lbl.StyleElements := Lbl.StyleElements - [seFont];
  Lbl.Font.Name := WizardForm.Font.Name;
  Lbl.Font.Size := 9;
  Lbl.Font.Color := InkMuted;
  Lbl.Caption := StatusText;

  BtnImg := CreatePrimaryBtnVisual(
    WizardForm.WelcomePage, 152, 226, 196, 40, BtnPath,
    CustomMessage('BtnInstallNow'), HeroPrimaryLabel
  );
  BtnImg.OnClick := @HeroPrimaryOnClick;
  HeroPrimaryLabel.OnClick := @HeroPrimaryOnClick;

  HeroPathLabel := TNewStaticText.Create(WizardForm);
  HeroPathLabel.Parent := WizardForm.WelcomePage;
  HeroPathLabel.AutoSize := False;
  HeroPathLabel.WordWrap := False;
  HeroPathLabel.SetBounds(ScaleX(24), ScaleY(328), ScaleX(324), ScaleY(20));
  HeroPathLabel.StyleElements := HeroPathLabel.StyleElements - [seFont];
  HeroPathLabel.Font.Name := WizardForm.Font.Name;
  HeroPathLabel.Font.Size := 9;
  HeroPathLabel.Font.Color := InkMuted;
  HeroPathLabel.Cursor := crHand;
  HeroPathLabel.ShowHint := True;
  HeroPathLabel.OnClick := @BrowseInstallDirOnClick;
  RefreshHeroPath;

  HeroCustomizeLink := TLabel.Create(WizardForm);
  HeroCustomizeLink.Parent := WizardForm.WelcomePage;
  HeroCustomizeLink.Transparent := True;
  HeroCustomizeLink.AutoSize := False;
  HeroCustomizeLink.Alignment := taRightJustify;
  HeroCustomizeLink.StyleElements := HeroCustomizeLink.StyleElements - [seFont];
  HeroCustomizeLink.Font.Name := WizardForm.Font.Name;
  HeroCustomizeLink.Font.Size := 9;
  HeroCustomizeLink.Font.Color := InkAccent;
  HeroCustomizeLink.Caption := CustomMessage('LinkCustomize');
  HeroCustomizeLink.SetBounds(ScaleX(350), ScaleY(328), ScaleX(126), ScaleY(20));
  HeroCustomizeLink.Cursor := crHand;
  HeroCustomizeLink.OnClick := @HeroCustomizeOnClick;

  // --- Page 2: Unified Custom Options Page (520 x 468) ---
  WizardForm.SelectComponentsLabel.Visible := False;

  CustomBackLink := TLabel.Create(WizardForm);
  CustomBackLink.Parent := WizardForm.SelectComponentsPage;
  CustomBackLink.Transparent := True;
  CustomBackLink.AutoSize := False;
  CustomBackLink.StyleElements := CustomBackLink.StyleElements - [seFont];
  CustomBackLink.Font.Name := WizardForm.Font.Name;
  CustomBackLink.Font.Size := 9;
  CustomBackLink.Font.Color := InkSecondary;
  CustomBackLink.Caption := CustomMessage('LinkBack');
  CustomBackLink.SetBounds(ScaleX(24), ScaleY(16), ScaleX(120), ScaleY(20));
  CustomBackLink.Cursor := crHand;
  CustomBackLink.OnClick := @CustomBackOnClick;

  Lbl := TNewStaticText.Create(WizardForm);
  Lbl.Parent := WizardForm.SelectComponentsPage;
  Lbl.AutoSize := False;
  Lbl.SetBounds(ScaleX(24), ScaleY(40), ScaleX(472), ScaleY(28));
  Lbl.StyleElements := Lbl.StyleElements - [seFont];
  Lbl.Font.Name := WizardForm.Font.Name;
  Lbl.Font.Size := 14;
  Lbl.Font.Style := [fsBold];
  Lbl.Font.Color := InkPrimary;
  Lbl.Caption := CustomMessage('LabelCustomTitle');

  DirLbl := TNewStaticText.Create(WizardForm);
  DirLbl.Parent := WizardForm.SelectComponentsPage;
  DirLbl.AutoSize := False;
  DirLbl.SetBounds(ScaleX(24), ScaleY(76), ScaleX(72), ScaleY(22));
  DirLbl.StyleElements := DirLbl.StyleElements - [seFont];
  DirLbl.Font.Name := WizardForm.Font.Name;
  DirLbl.Font.Size := 9;
  DirLbl.Font.Color := InkSecondary;
  DirLbl.Caption := CustomMessage('LabelInstallDir');

  BrowseW := WizardForm.CalculateButtonWidth([CustomMessage('BtnBrowse')]);
  if BrowseW < ScaleX(76) then
    BrowseW := ScaleX(76);

  CustomDirEdit := TNewEdit.Create(WizardForm);
  CustomDirEdit.Parent := WizardForm.SelectComponentsPage;
  CustomDirEdit.SetBounds(ScaleX(100), ScaleY(73), ScaleX(496) - ScaleX(100) - BrowseW - ScaleX(8), ScaleY(24));
  CustomDirEdit.Text := WizardForm.DirEdit.Text;

  CustomBrowseBtn := TNewButton.Create(WizardForm);
  CustomBrowseBtn.Parent := WizardForm.SelectComponentsPage;
  CustomBrowseBtn.Caption := CustomMessage('BtnBrowse');
  CustomBrowseBtn.SetBounds(ScaleX(496) - BrowseW, ScaleY(72), BrowseW, ScaleY(26));
  CustomBrowseBtn.OnClick := @BrowseInstallDirOnClick;

  WizardForm.TypesCombo.SetBounds(ScaleX(24), ScaleY(110), ScaleX(472), WizardForm.TypesCombo.Height);
  WizardForm.ComponentsList.SetBounds(ScaleX(24), ScaleY(144), ScaleX(472), ScaleY(190));

  CustomDesktopCheck := TNewCheckBox.Create(WizardForm);
  CustomDesktopCheck.Parent := WizardForm.SelectComponentsPage;
  CustomDesktopCheck.SetBounds(ScaleX(24), ScaleY(346), ScaleX(155), ScaleY(22));
  CustomDesktopCheck.Caption := CustomMessage('TaskDesktopIcon');
  CustomDesktopCheck.OnClick := @CustomTaskCheckOnClick;

  CustomAutostartCheck := TNewCheckBox.Create(WizardForm);
  CustomAutostartCheck.Parent := WizardForm.SelectComponentsPage;
  CustomAutostartCheck.SetBounds(ScaleX(182), ScaleY(346), ScaleX(155), ScaleY(22));
  CustomAutostartCheck.Caption := CustomMessage('TaskAutostart');
  CustomAutostartCheck.OnClick := @CustomTaskCheckOnClick;

  SpaceW := ScaleX(152);
  WizardForm.ComponentsDiskSpaceLabel.AutoSize := False;
  WizardForm.ComponentsDiskSpaceLabel.Alignment := taRightJustify;
  WizardForm.ComponentsDiskSpaceLabel.StyleElements := WizardForm.ComponentsDiskSpaceLabel.StyleElements - [seFont];
  WizardForm.ComponentsDiskSpaceLabel.Font.Color := InkMuted;
  WizardForm.ComponentsDiskSpaceLabel.SetBounds(ScaleX(496) - SpaceW, ScaleY(349), SpaceW, ScaleY(20));

  BtnImg := CreatePrimaryBtnVisual(
    WizardForm.SelectComponentsPage, 162, 400, 196, 40, BtnPath,
    CustomMessage('BtnStartInstall'), CustomPrimaryLabel
  );
  BtnImg.OnClick := @CustomPrimaryOnClick;
  CustomPrimaryLabel.OnClick := @CustomPrimaryOnClick;

  // --- Page 2.5: Preparing Page (500 x 370) ---
  PrepLogo := InitLogoImage(WizardForm.PreparingPage, 500, 68, IconPath);

  PrepTitleLabel := TNewStaticText.Create(WizardForm);
  PrepTitleLabel.Parent := WizardForm.PreparingPage;
  PrepTitleLabel.AutoSize := False;
  PrepTitleLabel.SetBounds(ScaleX(24), ScaleY(148), ScaleX(452), ScaleY(30));
  PrepTitleLabel.Alignment := taCenter;
  PrepTitleLabel.StyleElements := PrepTitleLabel.StyleElements - [seFont];
  PrepTitleLabel.Font.Name := WizardForm.Font.Name;
  PrepTitleLabel.Font.Size := 15;
  PrepTitleLabel.Font.Style := [fsBold];
  PrepTitleLabel.Font.Color := InkPrimary;
  PrepTitleLabel.Caption := CustomMessage('PrepTitle');

  PrepStatusLabel := TNewStaticText.Create(WizardForm);
  PrepStatusLabel.Parent := WizardForm.PreparingPage;
  PrepStatusLabel.AutoSize := False;
  PrepStatusLabel.SetBounds(ScaleX(24), ScaleY(190), ScaleX(452), ScaleY(22));
  PrepStatusLabel.Alignment := taCenter;
  PrepStatusLabel.StyleElements := PrepStatusLabel.StyleElements - [seFont];
  PrepStatusLabel.Font.Name := WizardForm.Font.Name;
  PrepStatusLabel.Font.Size := 9;
  PrepStatusLabel.Font.Color := InkMuted;
  PrepStatusLabel.Caption := CustomMessage('PrepChecking');

  // Fallback RestartManager prompt spacing (when locked apps require user choice)
  WizardForm.PreparingLabel.SetBounds(ScaleX(24), ScaleY(24), ScaleX(452), ScaleY(36));
  WizardForm.PreparingErrorBitmapImage.Left := ScaleX(24);
  WizardForm.PreparingErrorBitmapImage.Top := ScaleY(68);
  WizardForm.PreparingMemo.SetBounds(ScaleX(24), ScaleY(100), ScaleX(452), ScaleY(130));
  WizardForm.PreparingYesRadio.SetBounds(ScaleX(24), ScaleY(240), ScaleX(452), ScaleY(22));
  WizardForm.PreparingNoRadio.SetBounds(ScaleX(24), ScaleY(266), ScaleX(452), ScaleY(22));

  // --- Page 3: Progress Page (500 x 370) ---
  WizardForm.FilenameLabel.Visible := False;

  LogoImg := InitLogoImage(WizardForm.InstallingPage, 500, 68, IconPath);

  ProgTitleLabel := TNewStaticText.Create(WizardForm);
  ProgTitleLabel.Parent := WizardForm.InstallingPage;
  ProgTitleLabel.AutoSize := False;
  ProgTitleLabel.SetBounds(ScaleX(24), ScaleY(148), ScaleX(452), ScaleY(30));
  ProgTitleLabel.Alignment := taCenter;
  ProgTitleLabel.StyleElements := ProgTitleLabel.StyleElements - [seFont];
  ProgTitleLabel.Font.Name := WizardForm.Font.Name;
  ProgTitleLabel.Font.Size := 15;
  ProgTitleLabel.Font.Style := [fsBold];
  ProgTitleLabel.Font.Color := InkPrimary;
  if InstallKind = 1 then
    ProgTitleLabel.Caption := CustomMessage('ProgressUpdating')
  else
    ProgTitleLabel.Caption := CustomMessage('ProgressInstalling');

  WizardForm.ProgressGauge.SetBounds(ScaleX(75), ScaleY(202), ScaleX(350), ScaleY(10));

  WizardForm.StatusLabel.AutoSize := False;
  WizardForm.StatusLabel.Alignment := taCenter;
  WizardForm.StatusLabel.StyleElements := WizardForm.StatusLabel.StyleElements - [seFont];
  WizardForm.StatusLabel.Font.Color := InkMuted;
  WizardForm.StatusLabel.SetBounds(ScaleX(24), ScaleY(226), ScaleX(452), ScaleY(22));

  // --- Page 4: Finish Page (500 x 370) ---
  WizardForm.WizardBitmapImage2.Visible := False;
  WizardForm.FinishedHeadingLabel.Visible := False;
  WizardForm.FinishedLabel.Visible := False;
  ParkOffscreen(WizardForm.RunList);
  ParkOffscreen(WizardForm.YesRadio);
  ParkOffscreen(WizardForm.NoRadio);

  LogoImg := InitLogoImage(WizardForm.FinishedPage, 500, 44, IconPath);

  Lbl := TNewStaticText.Create(WizardForm);
  Lbl.Parent := WizardForm.FinishedPage;
  Lbl.AutoSize := False;
  Lbl.SetBounds(ScaleX(24), ScaleY(120), ScaleX(452), ScaleY(34));
  Lbl.Alignment := taCenter;
  Lbl.StyleElements := Lbl.StyleElements - [seFont];
  Lbl.Font.Name := WizardForm.Font.Name;
  Lbl.Font.Size := 18;
  Lbl.Font.Style := [fsBold];
  Lbl.Font.Color := InkPrimary;
  Lbl.Caption := CustomMessage('FinishHeading');

  FinishSubLabel := TNewStaticText.Create(WizardForm);
  FinishSubLabel.Parent := WizardForm.FinishedPage;
  FinishSubLabel.AutoSize := False;
  FinishSubLabel.SetBounds(ScaleX(24), ScaleY(160), ScaleX(452), ScaleY(24));
  FinishSubLabel.Alignment := taCenter;
  FinishSubLabel.StyleElements := FinishSubLabel.StyleElements - [seFont];
  FinishSubLabel.Font.Name := WizardForm.Font.Name;
  FinishSubLabel.Font.Size := 10;
  FinishSubLabel.Font.Color := InkSecondary;
  FinishSubLabel.Caption := CustomMessage('FinishBody');

  BtnImg := CreatePrimaryBtnVisual(
    WizardForm.FinishedPage, 152, 216, 196, 40, BtnPath,
    CustomMessage('BtnClose'), FinishPrimaryLabel
  );
  BtnImg.OnClick := @FinishPrimaryOnClick;
  FinishPrimaryLabel.OnClick := @FinishPrimaryOnClick;

  FinishCloseLink := TLabel.Create(WizardForm);
  FinishCloseLink.Parent := WizardForm.FinishedPage;
  FinishCloseLink.Transparent := True;
  FinishCloseLink.AutoSize := False;
  FinishCloseLink.Alignment := taCenter;
  FinishCloseLink.StyleElements := FinishCloseLink.StyleElements - [seFont];
  FinishCloseLink.Font.Name := WizardForm.Font.Name;
  FinishCloseLink.Font.Size := 9;
  FinishCloseLink.Font.Color := InkSecondary;
  FinishCloseLink.Caption := CustomMessage('BtnClose');
  FinishCloseLink.SetBounds(ScaleX(24), ScaleY(272), ScaleX(452), ScaleY(22));
  FinishCloseLink.Cursor := crHand;
  FinishCloseLink.OnClick := @FinishCloseOnClick;
end;

procedure InitializeUninstallProgressForm;
begin
  ApplySeamlessTitleBar(UninstallProgressForm.Handle);
end;

function ShouldSkipPage(PageID: Integer): Boolean;
begin
  Result := False;
  if (PageID = wpSelectDir) or (PageID = wpSelectTasks) then
    Result := True
  else if (PageID = wpSelectComponents) and (not CustomMode) then
    Result := True;
end;

procedure CurPageChanged(CurPageID: Integer);
var
  IsPrepPrompt: Boolean;
begin
  NavPending := False;
  ParkOffscreen(WizardForm.BackButton);
  ParkOffscreen(WizardForm.CancelButton);

  if CurPageID = wpWelcome then
  begin
    ResizeAndCenterWindow(500, 370);
    WizardForm.WizardBitmapImage.Visible := False;
    WizardForm.WelcomeLabel1.Visible := False;
    WizardForm.WelcomeLabel2.Visible := False;
    RefreshHeroPath;
    if InstallKind = 1 then
      HeroPrimaryLabel.Caption := CustomMessage('BtnUpgradeNow')
    else if InstallKind = 2 then
      HeroPrimaryLabel.Caption := CustomMessage('BtnReinstall')
    else
      HeroPrimaryLabel.Caption := CustomMessage('BtnInstallNow');
    ParkOffscreen(WizardForm.NextButton);
  end
  else if CurPageID = wpSelectComponents then
  begin
    ResizeAndCenterWindow(520, 468);
    WizardForm.SelectComponentsLabel.Visible := False;
    WizardForm.TypesCombo.SetBounds(ScaleX(24), ScaleY(110), ScaleX(472), WizardForm.TypesCombo.Height);
    WizardForm.ComponentsList.SetBounds(ScaleX(24), ScaleY(144), ScaleX(472), ScaleY(190));
    WizardForm.ComponentsDiskSpaceLabel.SetBounds(ScaleX(496) - ScaleX(152), ScaleY(349), ScaleX(152), ScaleY(20));
    if CustomDirEdit <> nil then
      CustomDirEdit.Text := WizardForm.DirEdit.Text;
    SyncTasksToUI;
    if CustomBrowseBtn <> nil then
      WizardForm.ActiveControl := CustomBrowseBtn;
    if InstallKind = 1 then
      CustomPrimaryLabel.Caption := CustomMessage('BtnStartUpgrade')
    else
      CustomPrimaryLabel.Caption := CustomMessage('BtnStartInstall');
    ParkOffscreen(WizardForm.NextButton);
  end
  else if CurPageID = wpPreparing then
  begin
    ResizeAndCenterWindow(500, 370);
    IsPrepPrompt := WizardForm.PreparingMemo.Visible or
                    WizardForm.PreparingYesRadio.Visible or
                    WizardForm.PreparingNoRadio.Visible or
                    WizardForm.PreparingErrorBitmapImage.Visible;
    if IsPrepPrompt then
    begin
      if PrepLogo <> nil then PrepLogo.Visible := False;
      if PrepTitleLabel <> nil then PrepTitleLabel.Visible := False;
      if PrepStatusLabel <> nil then PrepStatusLabel.Visible := False;
      WizardForm.NextButton.TabStop := True;
      WizardForm.NextButton.SetBounds(ScaleX(152), ScaleY(306), ScaleX(196), ScaleY(38));
      WizardForm.NextButton.BringToFront;
    end else begin
      if PrepLogo <> nil then PrepLogo.Visible := True;
      if PrepTitleLabel <> nil then PrepTitleLabel.Visible := True;
      if PrepStatusLabel <> nil then
      begin
        PrepStatusLabel.Caption := CustomMessage('PrepChecking');
        PrepStatusLabel.Visible := True;
      end;
      ParkOffscreen(WizardForm.NextButton);
    end;
    WizardForm.Update;
  end
  else if CurPageID = wpInstalling then
  begin
    ResizeAndCenterWindow(500, 370);
    WizardForm.FilenameLabel.Visible := False;
    ParkOffscreen(WizardForm.NextButton);
  end
  else if CurPageID = wpFinished then
  begin
    ResizeAndCenterWindow(500, 370);
    WizardForm.WizardBitmapImage2.Visible := False;
    WizardForm.FinishedHeadingLabel.Visible := False;
    WizardForm.FinishedLabel.Visible := False;
    WizardForm.RunList.Visible := False;
    ParkOffscreen(WizardForm.RunList);
    if WizardForm.RunList.Items.Count > 0 then
    begin
      FinishPrimaryLabel.Caption := WizardForm.RunList.ItemCaption[0];
      FinishCloseLink.Visible := True;
    end else begin
      FinishPrimaryLabel.Caption := CustomMessage('BtnClose');
      FinishCloseLink.Visible := False;
    end;
    if not WizardForm.YesRadio.Visible then
      ParkOffscreen(WizardForm.YesRadio)
    else begin
      WizardForm.YesRadio.SetBounds(ScaleX(120), ScaleY(268), ScaleX(260), ScaleY(22));
      FinishCloseLink.Visible := False;
    end;
    if not WizardForm.NoRadio.Visible then
      ParkOffscreen(WizardForm.NoRadio)
    else
      WizardForm.NoRadio.SetBounds(ScaleX(120), ScaleY(294), ScaleX(260), ScaleY(22));
    ParkOffscreen(WizardForm.NextButton);
  end else begin
    ResizeAndCenterWindow(500, 370);
    WizardForm.NextButton.TabStop := True;
    WizardForm.NextButton.SetBounds(ScaleX(152), ScaleY(306), ScaleX(196), ScaleY(38));
    WizardForm.NextButton.BringToFront;
  end;
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if CurPageID = wpSelectComponents then
  begin
    if CustomDirEdit <> nil then
      WizardForm.DirEdit.Text := RemoveBackslashUnlessRoot(Trim(CustomDirEdit.Text));
    if Trim(WizardForm.DirEdit.Text) = '' then
    begin
      NavPending := False;
      Result := False;
      Exit;
    end;
    SyncTasksFromUI;
    if (not WizardIsComponentSelected('capture')) and
       (not WizardIsComponentSelected('history')) and
       (not WizardIsComponentSelected('viewer')) and
       (not WizardIsComponentSelected('video')) and
       (not WizardIsComponentSelected('zip')) then
    begin
      MsgBox(CustomMessage('NeedModule'), mbError, MB_OK);
      NavPending := False;
      Result := False;
    end;
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  Manifest: String;
begin
  if CurStep = ssPostInstall then
  begin
    Manifest := '{' + #13#10 +
      '  "capture": ' + BoolToJson(WizardIsComponentSelected('capture')) + ',' + #13#10 +
      '  "history": ' + BoolToJson(WizardIsComponentSelected('history')) + ',' + #13#10 +
      '  "viewer": ' + BoolToJson(WizardIsComponentSelected('viewer')) + ',' + #13#10 +
      '  "video": ' + BoolToJson(WizardIsComponentSelected('video')) + ',' + #13#10 +
      '  "zip": ' + BoolToJson(WizardIsComponentSelected('zip')) + #13#10 +
      '}' + #13#10;
    SaveStringToFile(ExpandConstant('{app}\modules.json'), Manifest, False);
    CleanupLegacyAssociations;
    if WizardIsComponentSelected('zip\contextmenu') then
      RegisterZipContextMenu
    else
      UnregisterZipContextMenu;
    if not WizardIsComponentSelected('viewer\assoc') then
      CleanupOldAssociations;
    if not WizardIsComponentSelected('video\assoc') then
      CleanupOldVideoAssociations;
    if not WizardIsComponentSelected('zip\assoc') then
      CleanupOldZipAssociations;
    SHChangeNotify($08000000, 0, 0, 0);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
  begin
    UnregisterZipContextMenu;
    CleanupOldAssociations;
    CleanupOldVideoAssociations;
    CleanupOldZipAssociations;
    CleanupLegacyAssociations;
    SHChangeNotify($08000000, 0, 0, 0);
  end;
end;
