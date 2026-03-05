using System.Drawing;
using ClaudeUsageWidget.Services;

namespace ClaudeUsageWidget.UI;

/// <summary>
/// Debug console window that displays live debug messages.
/// </summary>
public class DebugConsole : Form
{
    private static readonly Color BackgroundColor = Color.FromArgb(25, 25, 25);
    private static readonly Color TextColor = Color.FromArgb(220, 220, 220);
    private static readonly Color InfoColor = Color.FromArgb(100, 180, 255);
    private static readonly Color WarningColor = Color.FromArgb(255, 200, 100);
    private static readonly Color ErrorColor = Color.FromArgb(255, 100, 100);
    private static readonly Color TimestampColor = Color.FromArgb(120, 120, 120);
    private static readonly Color CategoryColor = Color.FromArgb(180, 140, 255);
    private static readonly Color DetailsColor = Color.FromArgb(160, 160, 160);

    private readonly DebugService _debugService;
    private readonly RichTextBox _logBox;
    private readonly Button _clearButton;
    private readonly CheckBox _autoScrollCheckbox;
    private readonly Label _countLabel;
    private bool _autoScroll = true;

    public DebugConsole(DebugService debugService)
    {
        _debugService = debugService;

        // Form settings
        Text = "Claude Usage Widget - Debug Console";
        Size = new Size(800, 500);
        MinimumSize = new Size(500, 300);
        StartPosition = FormStartPosition.CenterScreen;
        BackColor = BackgroundColor;
        Icon = TrayIcon.IconGenerator.GenerateAppIcon();

        // Toolbar panel
        var toolbar = new Panel
        {
            Dock = DockStyle.Top,
            Height = 36,
            BackColor = Color.FromArgb(35, 35, 35),
            Padding = new Padding(8, 4, 8, 4)
        };

        _clearButton = new Button
        {
            Text = "Clear",
            FlatStyle = FlatStyle.Flat,
            BackColor = Color.FromArgb(60, 60, 60),
            ForeColor = TextColor,
            Size = new Size(60, 26),
            Location = new Point(8, 5),
            Cursor = Cursors.Hand
        };
        _clearButton.FlatAppearance.BorderColor = Color.FromArgb(80, 80, 80);
        _clearButton.Click += OnClearClick;
        toolbar.Controls.Add(_clearButton);

        _autoScrollCheckbox = new CheckBox
        {
            Text = "Auto-scroll",
            ForeColor = TextColor,
            Location = new Point(80, 8),
            AutoSize = true,
            Checked = true
        };
        _autoScrollCheckbox.CheckedChanged += (_, _) => _autoScroll = _autoScrollCheckbox.Checked;
        toolbar.Controls.Add(_autoScrollCheckbox);

        _countLabel = new Label
        {
            Text = "0 entries",
            ForeColor = TimestampColor,
            AutoSize = true,
            Location = new Point(toolbar.Width - 100, 10),
            Anchor = AnchorStyles.Top | AnchorStyles.Right
        };
        toolbar.Controls.Add(_countLabel);

        // Log display
        _logBox = new RichTextBox
        {
            Dock = DockStyle.Fill,
            BackColor = BackgroundColor,
            ForeColor = TextColor,
            Font = new Font("Consolas", 9.5f),
            ReadOnly = true,
            BorderStyle = BorderStyle.None,
            WordWrap = false,
            ScrollBars = RichTextBoxScrollBars.Both
        };
        Controls.Add(_logBox);

        Controls.Add(toolbar);

        // Load existing entries
        LoadExistingEntries();

        // Subscribe to new entries
        _debugService.EntryAdded += OnEntryAdded;

        // Clean up subscription when closed
        FormClosed += (_, _) => _debugService.EntryAdded -= OnEntryAdded;
    }

    /// <summary>
    /// Loads all existing debug entries into the console.
    /// </summary>
    private void LoadExistingEntries()
    {
        var entries = _debugService.GetEntries();
        foreach (var entry in entries)
        {
            AppendEntry(entry);
        }
        UpdateCountLabel();
    }

    /// <summary>
    /// Handles new debug entries being added.
    /// </summary>
    private void OnEntryAdded(DebugEntry entry)
    {
        if (InvokeRequired)
        {
            BeginInvoke(() => OnEntryAdded(entry));
            return;
        }

        AppendEntry(entry);
        UpdateCountLabel();
    }

    /// <summary>
    /// Appends a debug entry to the log box with color formatting.
    /// </summary>
    private void AppendEntry(DebugEntry entry)
    {
        var originalLength = _logBox.TextLength;

        // Timestamp
        AppendColored($"[{entry.Timestamp:HH:mm:ss}] ", TimestampColor);

        // Level indicator
        var (levelStr, levelColor) = entry.Level switch
        {
            DebugLevel.Error => ("ERR", ErrorColor),
            DebugLevel.Warning => ("WRN", WarningColor),
            _ => ("INF", InfoColor)
        };
        AppendColored($"[{levelStr}] ", levelColor);

        // Category
        AppendColored($"[{entry.Category}] ", CategoryColor);

        // Message
        AppendColored(entry.Message, TextColor);

        // Details (if present)
        if (!string.IsNullOrEmpty(entry.Details))
        {
            _logBox.AppendText(Environment.NewLine);
            AppendColored($"    {entry.Details}", DetailsColor);
        }

        _logBox.AppendText(Environment.NewLine);

        // Auto-scroll to bottom
        if (_autoScroll)
        {
            _logBox.SelectionStart = _logBox.TextLength;
            _logBox.ScrollToCaret();
        }
    }

    /// <summary>
    /// Appends colored text to the log box.
    /// </summary>
    private void AppendColored(string text, Color color)
    {
        _logBox.SelectionStart = _logBox.TextLength;
        _logBox.SelectionLength = 0;
        _logBox.SelectionColor = color;
        _logBox.AppendText(text);
        _logBox.SelectionColor = TextColor;
    }

    /// <summary>
    /// Updates the entry count label.
    /// </summary>
    private void UpdateCountLabel()
    {
        var count = _debugService.GetEntries().Count;
        _countLabel.Text = $"{count} entries";
    }

    /// <summary>
    /// Handles the clear button click.
    /// </summary>
    private void OnClearClick(object? sender, EventArgs e)
    {
        _debugService.Clear();
        _logBox.Clear();
        UpdateCountLabel();
    }

    /// <summary>
    /// Override to hide instead of close when X is clicked.
    /// </summary>
    protected override void OnFormClosing(FormClosingEventArgs e)
    {
        if (e.CloseReason == CloseReason.UserClosing)
        {
            e.Cancel = true;
            Hide();
        }
        else
        {
            base.OnFormClosing(e);
        }
    }
}
