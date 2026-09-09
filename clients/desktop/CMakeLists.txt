cmake_minimum_required(VERSION 3.21)
project(Headroom VERSION 0.1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

option(HEADROOM_WITH_KDE_TRAY "Use KDE StatusNotifierItem integration when available" ON)
option(HEADROOM_WITH_LAYER_SHELL "Use LayerShellQt popup placement when available" ON)

find_package(Qt6 6.6 REQUIRED COMPONENTS Quick QuickControls2 Widgets Network Svg Test)
if(CMAKE_SYSTEM_NAME STREQUAL "Linux")
    if(HEADROOM_WITH_LAYER_SHELL)
        find_package(LayerShellQt 6.6 QUIET)
    endif()
    if(HEADROOM_WITH_KDE_TRAY)
        find_package(KF6StatusNotifierItem QUIET)
    endif()
endif()

qt_standard_project_setup(REQUIRES 6.6)
if(Qt6_VERSION VERSION_GREATER_EQUAL 6.8)
    qt_policy(SET QTP0004 NEW)
endif()
if(MSVC)
    # Source and generated QML type-registration code contain Unicode labels.
    add_compile_options(/utf-8)
endif()

qt_add_library(headroom-core STATIC
    controller.cpp usage.cpp warning.cpp trayvisual.cpp startup.cpp appinfo.cpp popup.cpp)
target_link_libraries(headroom-core PUBLIC Qt6::Quick Qt6::Network Qt6::Svg)
target_include_directories(headroom-core PUBLIC ${CMAKE_CURRENT_SOURCE_DIR})
target_compile_definitions(headroom-core PUBLIC HEADROOM_VERSION="${PROJECT_VERSION}")
if(CMAKE_SYSTEM_NAME STREQUAL "Linux" AND HEADROOM_WITH_LAYER_SHELL AND LayerShellQt_FOUND)
    target_link_libraries(headroom-core PUBLIC LayerShellQt::Interface)
    target_compile_definitions(headroom-core PRIVATE HEADROOM_LAYER_SHELL)
endif()

set(headroom_platform_sources)
if(WIN32)
    set(HEADROOM_WINDOWS_ICON "${CMAKE_CURRENT_SOURCE_DIR}/windows/headroom.ico")
    set(HEADROOM_WINDOWS_MANIFEST "${CMAKE_CURRENT_SOURCE_DIR}/windows/headroom.manifest")
    if(MSVC)
        # MSVC creates manifest resource ID 1 itself. Supply our settings as a
        # linker merge input instead of compiling a second ID 1 into the RC.
        set(HEADROOM_MANIFEST_RESOURCE "")
    else()
        set(HEADROOM_MANIFEST_RESOURCE
            "CREATEPROCESS_MANIFEST_RESOURCE_ID RT_MANIFEST \"${HEADROOM_WINDOWS_MANIFEST}\"")
    endif()
    configure_file(windows/headroom.rc.in "${CMAKE_CURRENT_BINARY_DIR}/headroom.rc" @ONLY)
    list(APPEND headroom_platform_sources "${CMAKE_CURRENT_BINARY_DIR}/headroom.rc")
endif()
qt_add_executable(headroom WIN32 main.cpp ${headroom_platform_sources})
if(MSVC)
    target_link_options(headroom PRIVATE
        "/MANIFEST:EMBED"
        "/MANIFESTINPUT:${HEADROOM_WINDOWS_MANIFEST}")
endif()
if(CMAKE_SYSTEM_NAME STREQUAL "Linux" AND HEADROOM_WITH_KDE_TRAY AND KF6StatusNotifierItem_FOUND)
    target_link_libraries(headroom PRIVATE KF6::StatusNotifierItem)
    target_compile_definitions(headroom PRIVATE HEADROOM_KDE_TRAY)
endif()
target_link_libraries(headroom PRIVATE headroom-core Qt6::Widgets Qt6::QuickControls2 Qt6::Svg)

set_source_files_properties(qml/Theme.qml PROPERTIES QT_QML_SINGLETON_TYPE TRUE)
qt_add_qml_module(headroom URI Headroom VERSION 1.0 QML_FILES
    qml/Theme.qml qml/Main.qml qml/ActionButton.qml qml/ProviderCard.qml qml/Meter.qml qml/SettingsPanel.qml qml/StartupSettings.qml qml/AppInfoSettings.qml qml/DiagnosticsPanel.qml
    RESOURCES headroom.svg)
qt_add_resources(headroom provider-icons PREFIX "/provider-icons" BASE "../shared/provider-icons" FILES
    ../shared/provider-icons/claude.svg ../shared/provider-icons/codex.svg
    ../shared/provider-icons/cursor.svg ../shared/provider-icons/grok.svg)

include(CTest)
if(BUILD_TESTING)
    foreach(feature startup appinfo tray popup)
        qt_add_executable(headroom-${feature}-tests tests/test_${feature}.cpp)
        set_target_properties(headroom-${feature}-tests PROPERTIES WIN32_EXECUTABLE FALSE)
        target_link_libraries(headroom-${feature}-tests PRIVATE headroom-core Qt6::Test Qt6::Widgets)
        add_test(NAME headroom-${feature}-tests COMMAND headroom-${feature}-tests
            -o -,txt -o "${CMAKE_CURRENT_BINARY_DIR}/headroom-${feature}-tests.xml,junitxml")
        set_tests_properties(headroom-${feature}-tests PROPERTIES ENVIRONMENT "QT_QPA_PLATFORM=offscreen")
    endforeach()
    qt_add_resources(headroom-tray-tests provider-icons PREFIX "/provider-icons" BASE "../shared/provider-icons" FILES
        ../shared/provider-icons/claude.svg ../shared/provider-icons/codex.svg
        ../shared/provider-icons/cursor.svg ../shared/provider-icons/grok.svg)
    qt_add_executable(headroom-tests tests/test_usage.cpp)
    set_target_properties(headroom-tests PROPERTIES WIN32_EXECUTABLE FALSE)
    target_link_libraries(headroom-tests PRIVATE headroom-core Qt6::Test)
    qt_add_executable(headroom-ui-tests tests/test_ui.cpp)
    set_target_properties(headroom-ui-tests PROPERTIES WIN32_EXECUTABLE FALSE)
    target_link_libraries(headroom-ui-tests PRIVATE headroom-core Qt6::Test Qt6::Widgets Qt6::QuickControls2)
    target_compile_definitions(headroom-ui-tests PRIVATE SOURCE_DIR="${CMAKE_CURRENT_SOURCE_DIR}")
    qt_add_resources(headroom-ui-tests provider-icons PREFIX "/provider-icons" BASE "../shared/provider-icons" FILES
        ../shared/provider-icons/claude.svg ../shared/provider-icons/codex.svg
        ../shared/provider-icons/cursor.svg ../shared/provider-icons/grok.svg)
    add_test(NAME headroom-ui-tests COMMAND headroom-ui-tests
        -o -,txt -o "${CMAKE_CURRENT_BINARY_DIR}/headroom-ui-tests.xml,junitxml")
    set_tests_properties(headroom-ui-tests PROPERTIES ENVIRONMENT "QT_QPA_PLATFORM=offscreen;QT_QUICK_BACKEND=software")
    add_test(NAME headroom-tests COMMAND headroom-tests
        -o -,txt -o "${CMAKE_CURRENT_BINARY_DIR}/headroom-tests.xml,junitxml")
    set_tests_properties(headroom-tests PROPERTIES ENVIRONMENT "QT_QPA_PLATFORM=offscreen")
endif()

install(TARGETS headroom RUNTIME DESTINATION bin)
if(CMAKE_SYSTEM_NAME STREQUAL "Linux")
    install(FILES headroom.desktop DESTINATION share/applications)
    install(FILES headroom.svg DESTINATION share/icons/hicolor/scalable/apps)
endif()
install(FILES update-guide.html DESTINATION share/headroom)
