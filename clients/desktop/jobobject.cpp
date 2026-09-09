#include "managedserver.h"

#ifdef Q_OS_WIN
#define WIN32_LEAN_AND_MEAN
#include <windows.h>

bool ManagedServer::assignWindowsJob()
{
    if (!m_process || !m_process->processId()) return false;
    HANDLE job = CreateJobObjectW(nullptr, nullptr);
    if (!job) return false;
    JOBOBJECT_EXTENDED_LIMIT_INFORMATION limits{};
    limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
    if (!SetInformationJobObject(job, JobObjectExtendedLimitInformation, &limits, sizeof(limits))) {
        CloseHandle(job);
        return false;
    }
    HANDLE process = OpenProcess(PROCESS_SET_QUOTA | PROCESS_TERMINATE, FALSE,
                                 static_cast<DWORD>(m_process->processId()));
    if (!process) {
        CloseHandle(job);
        return false;
    }
    const bool assigned = AssignProcessToJobObject(job, process);
    CloseHandle(process);
    if (!assigned) {
        CloseHandle(job);
        return false;
    }
    m_job = job;
    return true;
}

void ManagedServer::closeWindowsJob()
{
    if (!m_job) return;
    CloseHandle(static_cast<HANDLE>(m_job));
    m_job = nullptr;
}
#else
bool ManagedServer::assignWindowsJob() { return true; }
void ManagedServer::closeWindowsJob() { m_job = nullptr; }
#endif
