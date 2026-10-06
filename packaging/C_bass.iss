; The Windows installer, made with Inno Setup:
;   iscc /DAppVersion=0.5.0 /DSource=C_bass.exe /DIcon=C_bass.ico packaging\C_bass.iss
; It installs for the current user alone, so it asks for no administrator password.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef Source
  #define Source "..\C_bass.exe"
#endif
#ifndef Icon
  #define Icon "..\C_bass.ico"
#endif

[Setup]
AppId={{6E0C3C0B-7B5E-4B4F-9A53-C5BA55000001}
AppName=C_bass
AppVersion={#AppVersion}
AppPublisher=Massimo Danieli
AppPublisherURL=https://github.com/MassimoDanieli/C_bass
AppSupportURL=https://github.com/MassimoDanieli/C_bass/issues
DefaultDirName={autopf}\C_bass
DefaultGroupName=C_bass
DisableProgramGroupPage=yes
DisableDirPage=auto
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=..
OutputBaseFilename=C_bass-windows-x64-setup
SetupIconFile={#Icon}
UninstallDisplayIcon={app}\C_bass.exe
UninstallDisplayName=C_bass
Compression=lzma2
SolidCompression=yes
WizardStyle=modern

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "italian"; MessagesFile: "compiler:Languages\Italian.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "{#Source}"; DestDir: "{app}"; DestName: "C_bass.exe"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\C_bass"; Filename: "{app}\C_bass.exe"
Name: "{autodesktop}\C_bass"; Filename: "{app}\C_bass.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\C_bass.exe"; Description: "{cm:LaunchProgram,C_bass}"; Flags: nowait postinstall skipifsilent
