@echo off
REM build_bridge.bat — Build media_bridge.dll from source and copy to bin/
REM
REM Requirements:
REM   1. Visual Studio 2022+ with "Desktop development with C++" workload
REM   2. Windows 10 SDK (10.0.19041 or later)
REM
REM Usage: build_bridge.bat [Debug|Release]
REM   Default: Release


set BUILD_TYPE=%~1
if "%BUILD_TYPE%"=="" set BUILD_TYPE=Release

set PROJECT_DIR=%~dp0..\..
set SOURCE_DIR=%PROJECT_DIR%\build\windows\bridge
set BIN_DIR=%PROJECT_DIR%\bin

echo === Timo Media Bridge Builder ===
echo Source: %SOURCE_DIR%\media_bridge.cpp
echo Output: %BIN_DIR%\media_bridge.dll
echo Build type: %BUILD_TYPE%
echo.

REM Check for CMake
where cmake >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo [ERROR] CMake not found. Install it from https://cmake.org/download/
    echo         or via: winget install Kitware.CMake
    exit /b 1
)

REM Check for Visual Studio (msbuild or cl)
where cl >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo [INFO] cl.exe not found in PATH. Looking for Visual Studio...

    REM Try to find vcvarsall.bat via vswhere
    if exist "%ProgramFiles(x86)%\Microsoft Visual Studio\Installer\vswhere.exe" (
        for /f "usebackq tokens=*" %%i in (`"%ProgramFiles(x86)%\Microsoft Visual Studio\Installer\vswhere.exe" -latest -property installationPath`) do (
            set VS_PATH=%%i
        )
    )

    if defined VS_PATH (
        call "!VS_PATH!\VC\Auxiliary\Build\vcvarsall.bat" x64
        if !ERRORLEVEL! neq 0 (
            echo [ERROR] Failed to set up Visual Studio environment.
            exit /b 1
        )
    ) else (
        echo [ERROR] Visual Studio 2022+ not found.
        echo         Install it from https://visualstudio.microsoft.com/vs/
        echo         Make sure to include "Desktop development with C++" workload.
        exit /b 1
    )
)

set BUILD_DIR=%PROJECT_DIR%\build\windows\bridge_build

REM Create build directory
if not exist "%BUILD_DIR%" mkdir "%BUILD_DIR%"

REM Configure with CMake
echo [1/3] Configuring CMake...
cmake -S "%PROJECT_DIR%\build\windows" -B "%BUILD_DIR%" -DCMAKE_BUILD_TYPE=%BUILD_TYPE%
if %ERRORLEVEL% neq 0 (
    echo [ERROR] CMake configuration failed.
    exit /b 1
)

REM Build the DLL
echo [2/3] Building media_bridge.dll...
cmake --build "%BUILD_DIR%" --config %BUILD_TYPE%
if %ERRORLEVEL% neq 0 (
    echo [ERROR] Build failed.
    exit /b 1
)

REM Ensure bin directory exists
if not exist "%BIN_DIR%" mkdir "%BIN_DIR%"

REM Copy DLL to bin/
echo [3/3] Copying to %BIN_DIR%\media_bridge.dll...
if "%BUILD_TYPE%"=="Debug" (
    copy /Y "%BUILD_DIR%\Debug\media_bridge.dll" "%BIN_DIR%\media_bridge.dll"
    copy /Y "%BUILD_DIR%\Debug\media_bridge.pdb" "%BIN_DIR%\media_bridge.pdb" >nul 2>nul
) else (
    copy /Y "%BUILD_DIR%\Release\media_bridge.dll" "%BIN_DIR%\media_bridge.dll"
)

echo.
echo === Done! ===
echo media_bridge.dll is ready at %BIN_DIR%\media_bridge.dll
echo Run it alongside timo.exe to enable native media playback controls.
