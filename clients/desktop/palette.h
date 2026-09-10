#pragma once

#include <QColor>
#include <QPalette>

inline QPalette headroomPalette()
{
    QPalette palette;
    palette.setColor(QPalette::Window, QColor("#282a36"));
    palette.setColor(QPalette::WindowText, QColor("#f8f8f2"));
    palette.setColor(QPalette::Base, QColor("#21222c"));
    palette.setColor(QPalette::AlternateBase, QColor("#303341"));
    palette.setColor(QPalette::Text, QColor("#f8f8f2"));
    palette.setColor(QPalette::Button, QColor("#44475a"));
    palette.setColor(QPalette::ButtonText, QColor("#f8f8f2"));
    palette.setColor(QPalette::Highlight, QColor("#bd93f9"));
    palette.setColor(QPalette::HighlightedText, QColor("#282a36"));
    palette.setColor(QPalette::ToolTipBase, QColor("#44475a"));
    palette.setColor(QPalette::ToolTipText, QColor("#f8f8f2"));
    palette.setColor(QPalette::Link, QColor("#8be9fd"));
    return palette;
}
