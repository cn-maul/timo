cmake_minimum_required(VERSION 3.16)
project(media_bridge
    VERSION 1.0.0
    DESCRIPTION "Timo GSMTC Native Bridge — C++/WinRT DLL"
    LANGUAGES CXX
)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# C++/WinRT requires the Windows 10 SDK (10.0.19041+).
# CMake will auto-detect it if Visual Studio or the Windows SDK is installed.
set(CMAKE_SYSTEM_VERSION 10.0.19041.0)

# Source files
set(SOURCES
    ${CMAKE_CURRENT_SOURCE_DIR}/bridge/media_bridge.cpp
)

# Header files (for IDE support)
set(HEADERS
    ${CMAKE_CURRENT_SOURCE_DIR}/bridge/media_bridge.h
)

# Output: media_bridge.dll in the build directory
add_library(media_bridge SHARED ${SOURCES} ${HEADERS})

# Define the DLL export macro
target_compile_definitions(media_bridge PRIVATE MEDIA_BRIDGE_EXPORTS)

# Link required Windows SDK libraries
target_link_libraries(media_bridge PRIVATE
    windowsapp
    runtimeobject
)

# Copy the built DLL to the project bin/ directory
set(BIN_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../../bin")

add_custom_command(TARGET media_bridge POST_BUILD
    COMMAND ${CMAKE_COMMAND} -E copy_if_different
        "$<TARGET_FILE:media_bridge>"
        "${BIN_DIR}/$<TARGET_FILE_NAME:media_bridge>"
    COMMENT "Copying media_bridge.dll to ${BIN_DIR}/"
)
