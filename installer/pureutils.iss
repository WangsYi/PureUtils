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
WizardStyle=modern
MinVersion=10.0
ChangesAssociations=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
CloseApplications=yes
RestartApplications=no
VersionInfoVersion={#AppVersion}.0
VersionInfoProductName=PureUtils
VersionInfoProductVersion={#AppVersion}

[Languages]
Name: "chinesesimplified"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[CustomMessages]
chinesesimplified.TypeFull=完整安装（PureShot + PureClip + PureImage + PureVideo + PureZip）
chinesesimplified.TypeShotClip=PureShot + PureClip（截图贴图 + 剪贴板历史）
chinesesimplified.TypeViewer=仅 PureImage（高性能看图 + 空格预览）
chinesesimplified.TypeVideo=仅 PureVideo（极速视频浏览）
chinesesimplified.TypeZip=仅 PureZip（压缩与解压管理）
chinesesimplified.TypeCustom=自定义选择组件
chinesesimplified.CompCore=PureUtils 核心框架（必需）
chinesesimplified.CompCapture=PureShot — 截图、标注与悬浮贴图
chinesesimplified.CompHistory=PureClip — 剪贴板历史管理
chinesesimplified.CompViewer=PureImage — 高性能看图与空格预览
chinesesimplified.CompTurbo=libjpeg-turbo 原生 JPEG 解码加速
chinesesimplified.CompAssoc=关联 40 种图片格式（注册系统“打开方式”与默认应用条目）
chinesesimplified.CompVideo=PureVideo — 极速视频浏览（内置 libmpv 硬件解码，支持 21 种格式与字幕）
chinesesimplified.CompVideoAssoc=关联 21 种视频格式（注册系统“打开方式”与默认应用条目）
chinesesimplified.CompZip=PureZip — 压缩与解压管理（浏览 / 智能解压 / AES-256 / 7z）
chinesesimplified.CompZipContext=注册资源管理器右键压缩 / 解压级联菜单
chinesesimplified.CompZipAssoc=关联 13 种压缩格式（注册系统“打开方式”与默认应用条目）
chinesesimplified.TaskAutostart=开机自动启动 PureUtils（后台托盘常驻）
chinesesimplified.LaunchViewer=打开 PureImage 看图
chinesesimplified.LaunchVideo=打开 PureVideo 视频
chinesesimplified.LaunchZip=打开 PureZip 压缩管理
chinesesimplified.NeedModule=请至少选择一个功能组件（PureShot、PureClip、PureImage、PureVideo 或 PureZip）。

english.TypeFull=Full installation (PureShot + PureClip + PureImage + PureVideo + PureZip)
english.TypeShotClip=PureShot + PureClip (Screenshot, Pins + Clipboard History)
english.TypeViewer=PureImage only (Fast Viewer + Explorer Space Preview)
english.TypeVideo=PureVideo only (Instant Video Browser)
english.TypeZip=PureZip only (Archive Manager, Smart Extract & Compression)
english.TypeCustom=Custom component selection
english.CompCore=PureUtils Core Runtime (Required)
english.CompCapture=PureShot — Screenshot, Annotation & Floating Pins
english.CompHistory=PureClip — Clipboard History Manager
english.CompViewer=PureImage — Fast Viewer & Explorer Space Preview
english.CompTurbo=libjpeg-turbo Native JPEG Scaling Acceleration
english.CompAssoc=Associate 40 Image Formats (Open With & Windows Default Apps Entry)
english.CompVideo=PureVideo — Fast Video Browser (Bundled libmpv Hardware Decoding, 21 Formats & Subtitles)
english.CompVideoAssoc=Associate 21 Video Formats (Open With & Windows Default Apps Entry)
english.CompZip=PureZip — Archive Manager (Browse, Smart Extract, AES-256, 7z)
english.CompZipContext=Register Windows Explorer Right-Click Archive Context Menu
english.CompZipAssoc=Associate 13 Archive Formats (Open With & Windows Default Apps Entry)
english.TaskAutostart=Start PureUtils automatically at sign-in
english.LaunchViewer=Launch PureImage Viewer
english.LaunchVideo=Launch PureVideo
english.LaunchZip=Launch PureZip Archive Manager
english.NeedModule=Please select at least one component (PureShot, PureClip, PureImage, PureVideo, or PureZip).

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
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "autostart"; Description: "{cm:TaskAutostart}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

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
Filename: "{app}\PureUtils.exe"; Parameters: ""; Description: "{cm:LaunchProgram,PureUtils}"; Flags: nowait postinstall skipifsilent; Components: capture or history
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
procedure SHChangeNotify(wEventId: LongInt; uFlags: Cardinal; dwItem1: NativeInt; dwItem2: NativeInt);
  external 'SHChangeNotify@shell32.dll stdcall';

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

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  ExePath, LegacyYnm, LegacyShot: String;
  ResultCode: Integer;
begin
  Result := '';
  ExePath := ExpandConstant('{app}\PureUtils.exe');
  if FileExists(ExePath) then
  begin
    Exec(ExePath, '--exit', '', SW_HIDE, ewNoWait, ResultCode);
    Sleep(800);
    Exec(ExpandConstant('{cmd}'), '/C taskkill /IM PureUtils.exe /F', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
    Sleep(250);
  end;
  LegacyYnm := ExpandConstant('{localappdata}\Programs\ynm3000\ynm3000.exe');
  if FileExists(LegacyYnm) then
  begin
    Exec(LegacyYnm, '--exit', '', SW_HIDE, ewNoWait, ResultCode);
    Sleep(500);
    Exec(ExpandConstant('{cmd}'), '/C taskkill /IM ynm3000.exe /F', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
    Sleep(200);
  end;
  LegacyShot := ExpandConstant('{localappdata}\Programs\ShotClip\ShotClip.exe');
  if FileExists(LegacyShot) then
  begin
    Exec(LegacyShot, '--exit', '', SW_HIDE, ewNoWait, ResultCode);
    Sleep(500);
    Exec(ExpandConstant('{cmd}'), '/C taskkill /IM ShotClip.exe /F', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
    Sleep(200);
  end;
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if CurPageID = wpSelectComponents then
  begin
    if (not WizardIsComponentSelected('capture')) and
       (not WizardIsComponentSelected('history')) and
       (not WizardIsComponentSelected('viewer')) and
       (not WizardIsComponentSelected('video')) and
       (not WizardIsComponentSelected('zip')) then
    begin
      MsgBox(CustomMessage('NeedModule'), mbError, MB_OK);
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
