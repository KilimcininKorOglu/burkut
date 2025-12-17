@echo off
setlocal enabledelayedexpansion

:: Burkut - Modern Download Manager
:: Build script for Windows (equivalent to Makefile)
:: Named after the golden eagle (berkut) in Turkish mythology

:: Colors (Windows 10+)
set "GREEN=[92m"
set "YELLOW=[93m"
set "RED=[91m"
set "CYAN=[96m"
set "NC=[0m"

:: Variables
set "BINARY_NAME=burkut"
set "BINARY_DIR=bin"
set "CMD_PATH=.\cmd\burkut"

:: Get version from git
for /f "tokens=*" %%i in ('git describe --tags --always --dirty 2^>nul') do set "VERSION=%%i"
if "%VERSION%"=="" set "VERSION=dev"

:: Get build time
for /f "tokens=*" %%i in ('powershell -command "Get-Date -Format 'yyyy-MM-dd_HH:mm:ss'"') do set "BUILD_TIME=%%i"

:: Get commit hash
for /f "tokens=*" %%i in ('git rev-parse --short HEAD 2^>nul') do set "COMMIT=%%i"
if "%COMMIT%"=="" set "COMMIT=unknown"

:: LDFLAGS
set "LDFLAGS=-ldflags "-s -w -X main.version=%VERSION% -X main.date=%BUILD_TIME% -X main.commit=%COMMIT%""

:: Parse command
if "%1"=="" goto :build
if "%1"=="help" goto :help
if "%1"=="-h" goto :help
if "%1"=="--help" goto :help
if "%1"=="init" goto :init
if "%1"=="build" goto :build
if "%1"=="build-linux" goto :build-linux
if "%1"=="build-linux-arm64" goto :build-linux-arm64
if "%1"=="build-linux-arm" goto :build-linux-arm
if "%1"=="build-windows" goto :build-windows
if "%1"=="build-windows-arm64" goto :build-windows-arm64
if "%1"=="build-darwin" goto :build-darwin
if "%1"=="build-darwin-arm64" goto :build-darwin-arm64
if "%1"=="build-freebsd" goto :build-freebsd
if "%1"=="build-all" goto :build-all
if "%1"=="test" goto :test
if "%1"=="test-unit" goto :test-unit
if "%1"=="test-integration" goto :test-integration
if "%1"=="test-cover" goto :test-cover
if "%1"=="test-bench" goto :test-bench
if "%1"=="lint" goto :lint
if "%1"=="fmt" goto :fmt
if "%1"=="vet" goto :vet
if "%1"=="check" goto :check
if "%1"=="clean" goto :clean
if "%1"=="run" goto :run
if "%1"=="install" goto :install
if "%1"=="deps" goto :deps
if "%1"=="deps-update" goto :deps-update
if "%1"=="generate" goto :generate
if "%1"=="checksums" goto :checksums
if "%1"=="release" goto :release
if "%1"=="version" goto :version

echo %RED%Unknown command: %1%NC%
echo Run 'build.bat help' for usage
exit /b 1

:: ==================== PROJECT INIT ====================

:init
echo %GREEN%Initializing Burkut project...%NC%
if not exist "go.mod" (
    go mod init github.com/kilimcininkoroglu/burkut
    echo   %GREEN%Created go.mod%NC%
) else (
    echo   %YELLOW%go.mod already exists%NC%
)
if not exist "cmd\burkut" mkdir "cmd\burkut"
if not exist "internal\cli" mkdir "internal\cli"
if not exist "internal\download" mkdir "internal\download"
if not exist "internal\engine" mkdir "internal\engine"
if not exist "internal\protocol" mkdir "internal\protocol"
if not exist "internal\storage" mkdir "internal\storage"
if not exist "internal\ui" mkdir "internal\ui"
if not exist "internal\hooks" mkdir "internal\hooks"
if not exist "pkg\burkut" mkdir "pkg\burkut"
echo   %GREEN%Created directory structure%NC%
if not exist "cmd\burkut\main.go" (
    (
        echo package main
        echo.
        echo import "fmt"
        echo.
        echo var ^(
        echo 	version = "dev"
        echo 	date    = "unknown"
        echo 	commit  = "unknown"
        echo ^)
        echo.
        echo func main^(^) {
        echo 	fmt.Printf^("Burkut %%s ^(%%s^) built on %%s\n", version, commit, date^)
        echo }
    ) > "cmd\burkut\main.go"
    echo   %GREEN%Created cmd\burkut\main.go%NC%
)
echo %GREEN%Burkut project initialized!%NC%
goto :eof

:: ==================== BUILD TARGETS ====================

:build
echo %GREEN%Building %BINARY_NAME% for Windows (amd64)...%NC%
if not exist "%BINARY_DIR%" mkdir "%BINARY_DIR%"
set CGO_ENABLED=0
go build %LDFLAGS% -o "%BINARY_DIR%\%BINARY_NAME%-windows-amd64.exe" %CMD_PATH%
if errorlevel 1 (
    echo %RED%Build failed%NC%
    exit /b 1
)
echo   %GREEN%Created: %BINARY_DIR%\%BINARY_NAME%-windows-amd64.exe%NC%
goto :eof

:: ==================== CROSS-COMPILATION ====================

:build-linux
echo %GREEN%Building %BINARY_NAME% for Linux (amd64)...%NC%
if not exist "%BINARY_DIR%" mkdir "%BINARY_DIR%"
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=0
go build %LDFLAGS% -o "%BINARY_DIR%\%BINARY_NAME%-linux-amd64" %CMD_PATH%
if errorlevel 1 (
    echo %RED%Build failed%NC%
    set GOOS=
    set GOARCH=
    exit /b 1
)
set GOOS=
set GOARCH=
echo   %GREEN%Created: %BINARY_DIR%\%BINARY_NAME%-linux-amd64%NC%
goto :eof

:build-linux-arm64
echo %GREEN%Building %BINARY_NAME% for Linux (arm64)...%NC%
if not exist "%BINARY_DIR%" mkdir "%BINARY_DIR%"
set GOOS=linux
set GOARCH=arm64
set CGO_ENABLED=0
go build %LDFLAGS% -o "%BINARY_DIR%\%BINARY_NAME%-linux-arm64" %CMD_PATH%
if errorlevel 1 (
    echo %RED%Build failed%NC%
    set GOOS=
    set GOARCH=
    exit /b 1
)
set GOOS=
set GOARCH=
echo   %GREEN%Created: %BINARY_DIR%\%BINARY_NAME%-linux-arm64%NC%
goto :eof

:build-linux-arm
echo %GREEN%Building %BINARY_NAME% for Linux (arm)...%NC%
if not exist "%BINARY_DIR%" mkdir "%BINARY_DIR%"
set GOOS=linux
set GOARCH=arm
set CGO_ENABLED=0
go build %LDFLAGS% -o "%BINARY_DIR%\%BINARY_NAME%-linux-arm" %CMD_PATH%
if errorlevel 1 (
    echo %RED%Build failed%NC%
    set GOOS=
    set GOARCH=
    exit /b 1
)
set GOOS=
set GOARCH=
echo   %GREEN%Created: %BINARY_DIR%\%BINARY_NAME%-linux-arm%NC%
goto :eof

:build-windows
echo %GREEN%Building %BINARY_NAME% for Windows (amd64)...%NC%
if not exist "%BINARY_DIR%" mkdir "%BINARY_DIR%"
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build %LDFLAGS% -o "%BINARY_DIR%\%BINARY_NAME%-windows-amd64.exe" %CMD_PATH%
if errorlevel 1 (
    echo %RED%Build failed%NC%
    set GOOS=
    set GOARCH=
    exit /b 1
)
set GOOS=
set GOARCH=
echo   %GREEN%Created: %BINARY_DIR%\%BINARY_NAME%-windows-amd64.exe%NC%
goto :eof

:build-windows-arm64
echo %GREEN%Building %BINARY_NAME% for Windows (arm64)...%NC%
if not exist "%BINARY_DIR%" mkdir "%BINARY_DIR%"
set GOOS=windows
set GOARCH=arm64
set CGO_ENABLED=0
go build %LDFLAGS% -o "%BINARY_DIR%\%BINARY_NAME%-windows-arm64.exe" %CMD_PATH%
if errorlevel 1 (
    echo %RED%Build failed%NC%
    set GOOS=
    set GOARCH=
    exit /b 1
)
set GOOS=
set GOARCH=
echo   %GREEN%Created: %BINARY_DIR%\%BINARY_NAME%-windows-arm64.exe%NC%
goto :eof

:build-darwin
echo %GREEN%Building %BINARY_NAME% for macOS (amd64)...%NC%
if not exist "%BINARY_DIR%" mkdir "%BINARY_DIR%"
set GOOS=darwin
set GOARCH=amd64
set CGO_ENABLED=0
go build %LDFLAGS% -o "%BINARY_DIR%\%BINARY_NAME%-darwin-amd64" %CMD_PATH%
if errorlevel 1 (
    echo %RED%Build failed%NC%
    set GOOS=
    set GOARCH=
    exit /b 1
)
set GOOS=
set GOARCH=
echo   %GREEN%Created: %BINARY_DIR%\%BINARY_NAME%-darwin-amd64%NC%
goto :eof

:build-darwin-arm64
echo %GREEN%Building %BINARY_NAME% for macOS (arm64/Apple Silicon)...%NC%
if not exist "%BINARY_DIR%" mkdir "%BINARY_DIR%"
set GOOS=darwin
set GOARCH=arm64
set CGO_ENABLED=0
go build %LDFLAGS% -o "%BINARY_DIR%\%BINARY_NAME%-darwin-arm64" %CMD_PATH%
if errorlevel 1 (
    echo %RED%Build failed%NC%
    set GOOS=
    set GOARCH=
    exit /b 1
)
set GOOS=
set GOARCH=
echo   %GREEN%Created: %BINARY_DIR%\%BINARY_NAME%-darwin-arm64%NC%
goto :eof

:build-freebsd
echo %GREEN%Building %BINARY_NAME% for FreeBSD (amd64)...%NC%
if not exist "%BINARY_DIR%" mkdir "%BINARY_DIR%"
set GOOS=freebsd
set GOARCH=amd64
set CGO_ENABLED=0
go build %LDFLAGS% -o "%BINARY_DIR%\%BINARY_NAME%-freebsd-amd64" %CMD_PATH%
if errorlevel 1 (
    echo %RED%Build failed%NC%
    set GOOS=
    set GOARCH=
    exit /b 1
)
set GOOS=
set GOARCH=
echo   %GREEN%Created: %BINARY_DIR%\%BINARY_NAME%-freebsd-amd64%NC%
goto :eof

:build-all
echo %GREEN%Building %BINARY_NAME% for all platforms...%NC%
echo.
call :build-linux
if errorlevel 1 exit /b 1
call :build-linux-arm64
if errorlevel 1 exit /b 1
call :build-linux-arm
if errorlevel 1 exit /b 1
call :build-windows
if errorlevel 1 exit /b 1
call :build-windows-arm64
if errorlevel 1 exit /b 1
call :build-darwin
if errorlevel 1 exit /b 1
call :build-darwin-arm64
if errorlevel 1 exit /b 1
call :build-freebsd
if errorlevel 1 exit /b 1
echo.
echo %GREEN%All platform binaries built successfully!%NC%
echo Binaries available in %BINARY_DIR%\
dir /b "%BINARY_DIR%"
goto :eof

:: ==================== TEST TARGETS ====================

:test
call :test-unit
echo %GREEN%All tests completed%NC%
goto :eof

:test-unit
echo %CYAN%Running unit tests...%NC%
go test -v -short -coverprofile=coverage.out ./...
if errorlevel 1 (
    echo %RED%Unit tests failed%NC%
    exit /b 1
)
go tool cover -html=coverage.out -o coverage.html
echo %GREEN%Unit tests passed. Coverage report: coverage.html%NC%
goto :eof

:test-integration
echo %CYAN%Running integration tests...%NC%
go test -v -tags=integration ./...
if errorlevel 1 (
    echo %RED%Integration tests failed%NC%
    exit /b 1
)
echo %GREEN%Integration tests passed%NC%
goto :eof

:test-cover
echo %CYAN%Running tests with coverage...%NC%
go test -v -coverprofile=coverage.out -covermode=atomic ./...
if errorlevel 1 (
    echo %RED%Tests failed%NC%
    exit /b 1
)
go tool cover -func=coverage.out
echo %GREEN%Coverage report generated%NC%
goto :eof

:test-bench
echo %CYAN%Running benchmarks...%NC%
go test -bench=. -benchmem ./...
if errorlevel 1 (
    echo %RED%Benchmarks failed%NC%
    exit /b 1
)
echo %GREEN%Benchmarks completed%NC%
goto :eof

:: ==================== CODE QUALITY ====================

:lint
echo %CYAN%Running linter...%NC%
golangci-lint run ./...
if errorlevel 1 (
    echo %RED%Linting failed%NC%
    exit /b 1
)
echo %GREEN%Linting passed%NC%
goto :eof

:fmt
echo %CYAN%Formatting code...%NC%
gofmt -s -w .
go mod tidy
echo %GREEN%Code formatted%NC%
goto :eof

:vet
echo %CYAN%Running go vet...%NC%
go vet ./...
if errorlevel 1 (
    echo %RED%go vet found issues%NC%
    exit /b 1
)
echo %GREEN%go vet passed%NC%
goto :eof

:check
echo %CYAN%Running all checks...%NC%
call :fmt
call :vet
call :lint
if errorlevel 1 exit /b 1
call :test
if errorlevel 1 exit /b 1
echo %GREEN%All checks passed%NC%
goto :eof

:: ==================== CLEAN ====================

:clean
echo %CYAN%Cleaning build artifacts...%NC%
if exist "%BINARY_DIR%" rmdir /s /q "%BINARY_DIR%"
if exist "coverage.out" del "coverage.out"
if exist "coverage.html" del "coverage.html"
if exist "%BINARY_NAME%.exe" del "%BINARY_NAME%.exe"
echo %GREEN%Cleaned%NC%
goto :eof

:: ==================== RUN & INSTALL ====================

:run
call :build
if errorlevel 1 exit /b 1
echo %GREEN%Running %BINARY_NAME%...%NC%
"%BINARY_DIR%\%BINARY_NAME%-windows-amd64.exe"
goto :eof

:install
call :build
if errorlevel 1 exit /b 1
echo %CYAN%Installing %BINARY_NAME% to GOPATH\bin...%NC%
copy "%BINARY_DIR%\%BINARY_NAME%-windows-amd64.exe" "%GOPATH%\bin\%BINARY_NAME%.exe"
echo %GREEN%Installed: %GOPATH%\bin\%BINARY_NAME%.exe%NC%
goto :eof

:: ==================== RELEASE ====================

:checksums
echo %CYAN%Generating checksums...%NC%
cd "%BINARY_DIR%"
powershell -command "Get-ChildItem -Filter '%BINARY_NAME%-*' | ForEach-Object { $hash = (Get-FileHash $_.Name -Algorithm SHA256).Hash.ToLower(); \"$hash  $($_.Name)\" } | Out-File -Encoding ASCII checksums.txt"
cd ..
echo %GREEN%Checksums saved to %BINARY_DIR%\checksums.txt%NC%
type "%BINARY_DIR%\checksums.txt"
goto :eof

:release
echo %GREEN%Building release...%NC%
call :clean
call :build-all
if errorlevel 1 exit /b 1
call :checksums
echo.
echo %GREEN%Release artifacts ready in %BINARY_DIR%\%NC%
goto :eof

:: ==================== DEPENDENCIES ====================

:deps
echo %CYAN%Downloading dependencies...%NC%
go mod download
go mod verify
echo %GREEN%Dependencies downloaded and verified%NC%
goto :eof

:deps-update
echo %CYAN%Updating dependencies...%NC%
go get -u ./...
go mod tidy
echo %GREEN%Dependencies updated%NC%
goto :eof

:: ==================== GENERATE ====================

:generate
echo %CYAN%Generating code...%NC%
go generate ./...
echo %GREEN%Code generated%NC%
goto :eof

:: ==================== VERSION ====================

:version
echo.
echo %CYAN%%BINARY_NAME% build information:%NC%
echo   Version:    %VERSION%
echo   Commit:     %COMMIT%
echo   Build Time: %BUILD_TIME%
echo.
goto :eof

:: ==================== HELP ====================

:help
echo.
echo %GREEN%Burkut - Modern Download Manager%NC%
echo %GREEN%=================================%NC%
echo %CYAN%Named after the golden eagle (berkut) in Turkish mythology%NC%
echo.
echo Usage: build.bat [command]
echo.
echo %YELLOW%Project setup:%NC%
echo   init               Initialize project structure and go.mod
echo.
echo %YELLOW%Build targets:%NC%
echo   build              Build for Windows (default)
echo   build-all          Build for all supported platforms
echo.
echo %YELLOW%Cross-compilation targets:%NC%
echo   build-linux        Build for Linux (amd64)
echo   build-linux-arm64  Build for Linux (arm64)
echo   build-linux-arm    Build for Linux (arm)
echo   build-windows      Build for Windows (amd64)
echo   build-windows-arm64 Build for Windows (arm64)
echo   build-darwin       Build for macOS (amd64/Intel)
echo   build-darwin-arm64 Build for macOS (arm64/Apple Silicon)
echo   build-freebsd      Build for FreeBSD (amd64)
echo.
echo %YELLOW%Test targets:%NC%
echo   test               Run all tests
echo   test-unit          Run unit tests with coverage
echo   test-integration   Run integration tests
echo   test-cover         Run tests with coverage report
echo   test-bench         Run benchmarks
echo.
echo %YELLOW%Code quality:%NC%
echo   lint               Run golangci-lint
echo   fmt                Format code and tidy modules
echo   vet                Run go vet
echo   check              Run fmt, vet, lint, and test
echo.
echo %YELLOW%Run ^& Install:%NC%
echo   run                Build and run
echo   install            Install to GOPATH\bin
echo.
echo %YELLOW%Release:%NC%
echo   release            Build all platforms + checksums
echo   checksums          Generate SHA256 checksums
echo.
echo %YELLOW%Other:%NC%
echo   deps               Download and verify dependencies
echo   deps-update        Update dependencies
echo   generate           Run go generate
echo   version            Show version info
echo   clean              Remove build artifacts
echo   help               Show this help message
echo.
echo %CYAN%Binary naming: burkut-{os}-{arch}[.exe]%NC%
echo Examples: burkut-linux-amd64, burkut-windows-amd64.exe, burkut-darwin-arm64
echo.
goto :eof
