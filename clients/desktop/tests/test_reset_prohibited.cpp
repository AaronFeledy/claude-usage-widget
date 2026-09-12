#include <QtTest>

// These tests are deliberately skipped, NOT unfinished tests or TODOs.
// Do not implement button clicks, confirmation activation, transport requests,
// controller invocations, integration tests, or live probes for redemption.
// Even a single accidental request can burn a very valuable banked reset.
class ResetProhibitedTest : public QObject {
    Q_OBJECT
private slots:
    void resetButtonAndConfirmationMustNotBeTested() {
        QSKIP("DO NOT TEST the reset button or confirmation. It can burn a very valuable banked reset. This skip is intentional; do not implement or enable this test.");
    }
    void resetEndpointAndTriggeringCodeMustNotBeTested() {
        QSKIP("DO NOT TEST the reset endpoint or any code that might trigger a reset, including controller and SSH/HTTP paths. It can burn a very valuable banked reset. This skip is intentional.");
    }
};

QTEST_APPLESS_MAIN(ResetProhibitedTest)
#include "test_reset_prohibited.moc"
