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
    void resetLatchAndDelayedRefreshMustNotBeTestedThroughRedemption() {
        QSKIP("DO NOT TEST the reset latch or delayed refresh by clicking a reset button, confirming redemption, invoking the reset controller, or calling the endpoint, even with a mocked transport. A reset is very valuable. Review this flow statically; this skip must not be implemented or enabled.");
    }
    void resetTransportMustNeverBeRetriedOrTested() {
        QSKIP("DO NOT TEST or add retry behavior to reset submission, upload replay, SSH forwarding, or the provider POST. Do not simulate failed reset requests or consume a reset to verify that retries are disabled. This intentional skip must remain unimplemented and disabled.");
    }
};

QTEST_APPLESS_MAIN(ResetProhibitedTest)
#include "test_reset_prohibited.moc"
