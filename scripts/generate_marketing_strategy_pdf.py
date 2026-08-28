#!/usr/bin/env python3
"""Generate the WA Blast Managed Care marketing strategy PDF."""

from pathlib import Path

from reportlab.lib import colors
from reportlab.lib.enums import TA_CENTER, TA_LEFT
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.lib.units import mm
from reportlab.platypus import (
    KeepTogether,
    LongTable,
    PageBreak,
    Paragraph,
    SimpleDocTemplate,
    Spacer,
    Table,
    TableStyle,
)


ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "docs" / "strategi-marketing-wa-blast-managed-care.pdf"

GREEN_DARK = colors.HexColor("#0B2C21")
GREEN = colors.HexColor("#176744")
GREEN_MID = colors.HexColor("#20A565")
GREEN_LIGHT = colors.HexColor("#EDF9F2")
GREEN_PALE = colors.HexColor("#F5FAF7")
INK = colors.HexColor("#17211D")
MUTED = colors.HexColor("#65776F")
LINE = colors.HexColor("#D9E6DF")
WHITE = colors.white


styles = getSampleStyleSheet()
styles.add(ParagraphStyle(name="CoverKicker", parent=styles["Normal"], fontName="Helvetica-Bold", fontSize=9, leading=12, textColor=colors.HexColor("#9AF1BC"), spaceAfter=18))
styles.add(ParagraphStyle(name="CoverTitle", parent=styles["Title"], fontName="Helvetica-Bold", fontSize=34, leading=37, textColor=WHITE, alignment=TA_LEFT, spaceAfter=18))
styles.add(ParagraphStyle(name="CoverSub", parent=styles["Normal"], fontSize=15, leading=22, textColor=colors.HexColor("#D8F7E5"), spaceAfter=12))
styles.add(ParagraphStyle(name="Kicker", parent=styles["Normal"], fontName="Helvetica-Bold", fontSize=8, leading=11, textColor=GREEN_MID, spaceAfter=4))
styles.add(ParagraphStyle(name="H1x", parent=styles["Heading1"], fontName="Helvetica-Bold", fontSize=19, leading=23, textColor=GREEN_DARK, spaceAfter=11))
styles.add(ParagraphStyle(name="H2x", parent=styles["Heading2"], fontName="Helvetica-Bold", fontSize=12, leading=15, textColor=GREEN, spaceBefore=10, spaceAfter=6))
styles.add(ParagraphStyle(name="Bodyx", parent=styles["BodyText"], fontName="Helvetica", fontSize=9.5, leading=14, textColor=INK, spaceAfter=6))
styles.add(ParagraphStyle(name="Lead", parent=styles["BodyText"], fontSize=11.5, leading=17, textColor=colors.HexColor("#314A40"), spaceAfter=10))
styles.add(ParagraphStyle(name="Small", parent=styles["BodyText"], fontSize=8, leading=11, textColor=MUTED, spaceAfter=4))
styles.add(ParagraphStyle(name="Callout", parent=styles["BodyText"], fontSize=10, leading=15, textColor=GREEN_DARK, leftIndent=7, rightIndent=7, spaceBefore=5, spaceAfter=5))
styles.add(ParagraphStyle(name="Quote", parent=styles["BodyText"], fontSize=10, leading=15, textColor=GREEN_DARK, leftIndent=8, rightIndent=8, spaceBefore=5, spaceAfter=8))
styles.add(ParagraphStyle(name="Bulletx", parent=styles["BodyText"], fontSize=9.3, leading=13.5, leftIndent=13, firstLineIndent=-8, bulletIndent=5, textColor=INK, spaceAfter=3))
styles.add(ParagraphStyle(name="Cell", parent=styles["BodyText"], fontSize=8.2, leading=11, textColor=INK))
styles.add(ParagraphStyle(name="CellHead", parent=styles["BodyText"], fontName="Helvetica-Bold", fontSize=8.2, leading=10, textColor=WHITE))


def p(text, style="Bodyx"):
    return Paragraph(text, styles[style])


def bullets(items):
    return [Paragraph(f"- {item}", styles["Bulletx"]) for item in items]


def heading(kicker, title):
    return [p(kicker.upper(), "Kicker"), p(title, "H1x"), Table([[""]], colWidths=[180 * mm], rowHeights=[1.2 * mm], style=TableStyle([("BACKGROUND", (0, 0), (-1, -1), LINE), ("LINEBELOW", (0, 0), (-1, -1), 0, LINE)])), Spacer(1, 4 * mm)]


def callout(text, dark=False):
    bg = GREEN_DARK if dark else GREEN_LIGHT
    fg_style = ParagraphStyle("CalloutTemp", parent=styles["Callout"], textColor=WHITE if dark else GREEN_DARK)
    return Table([[Paragraph(text, fg_style)]], colWidths=[180 * mm], style=TableStyle([
        ("BACKGROUND", (0, 0), (-1, -1), bg),
        ("BOX", (0, 0), (-1, -1), 0.7, GREEN_MID),
        ("LEFTPADDING", (0, 0), (-1, -1), 9), ("RIGHTPADDING", (0, 0), (-1, -1), 9),
        ("TOPPADDING", (0, 0), (-1, -1), 8), ("BOTTOMPADDING", (0, 0), (-1, -1), 8),
    ]))


def cards(items, columns=2):
    rows = []
    for i in range(0, len(items), columns):
        row = []
        for title, body in items[i:i + columns]:
            row.append([p(title, "H2x"), p(body, "Bodyx")])
        while len(row) < columns:
            row.append("")
        rows.append(row)
    table = Table(rows, colWidths=[(180 * mm - (columns - 1) * 4 * mm) / columns] * columns, hAlign="LEFT")
    table.setStyle(TableStyle([
        ("VALIGN", (0, 0), (-1, -1), "TOP"), ("BACKGROUND", (0, 0), (-1, -1), GREEN_PALE),
        ("BOX", (0, 0), (-1, -1), 0.6, LINE), ("INNERGRID", (0, 0), (-1, -1), 4, WHITE),
        ("LEFTPADDING", (0, 0), (-1, -1), 9), ("RIGHTPADDING", (0, 0), (-1, -1), 9),
        ("TOPPADDING", (0, 0), (-1, -1), 7), ("BOTTOMPADDING", (0, 0), (-1, -1), 7),
    ]))
    return table


def data_table(headers, rows, widths):
    data = [[p(x, "CellHead") for x in headers]]
    data.extend([[p(str(x), "Cell") for x in row] for row in rows])
    table = LongTable(data, colWidths=widths, repeatRows=1, hAlign="LEFT")
    table.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), GREEN), ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [WHITE, GREEN_PALE]),
        ("LINEBELOW", (0, 1), (-1, -1), 0.4, LINE),
        ("LEFTPADDING", (0, 0), (-1, -1), 6), ("RIGHTPADDING", (0, 0), (-1, -1), 6),
        ("TOPPADDING", (0, 0), (-1, -1), 6), ("BOTTOMPADDING", (0, 0), (-1, -1), 6),
    ]))
    return table


def section(story, kicker, title):
    if story:
        story.append(PageBreak())
    story.extend(heading(kicker, title))


def draw_page(canvas, doc):
    width, height = A4
    canvas.saveState()
    if doc.page == 1:
        canvas.setFillColor(GREEN_DARK)
        canvas.rect(0, 0, width, height, stroke=0, fill=1)
        canvas.setFillColor(colors.HexColor("#168756"))
        canvas.circle(width - 20 * mm, height - 24 * mm, 50 * mm, stroke=0, fill=1)
    else:
        canvas.setStrokeColor(LINE)
        canvas.line(15 * mm, 13 * mm, width - 15 * mm, 13 * mm)
        canvas.setFont("Helvetica", 7.5)
        canvas.setFillColor(MUTED)
        canvas.drawString(15 * mm, 8.5 * mm, "WA Blast Managed Care")
        canvas.drawRightString(width - 15 * mm, 8.5 * mm, f"Strategi Marketing  |  {doc.page}")
    canvas.restoreState()


def build():
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    story = []

    story += [Spacer(1, 25 * mm), p("DOKUMEN STRATEGI MARKETING", "CoverKicker"), p("WA Blast<br/>Managed Care", "CoverTitle"), p("Strategi mengubah pengguna mandiri menjadi pelanggan bulanan tanpa mengurangi kontrol server yang sudah mereka miliki.", "CoverSub"), Spacer(1, 43 * mm)]
    metrics = Table([
        [p("<b>Rp199K</b><br/>harga normal per bulan", "CoverSub"), p("<b>30 Hari</b><br/>trial pelanggan lama", "CoverSub"), p("<b>20 Member</b><br/>batch pertama", "CoverSub")]
    ], colWidths=[55 * mm] * 3, style=TableStyle([("BOX", (0, 0), (-1, -1), .6, colors.HexColor("#78BE94")), ("INNERGRID", (0, 0), (-1, -1), .4, colors.HexColor("#78BE94")), ("BACKGROUND", (0, 0), (-1, -1), colors.HexColor("#124937")), ("VALIGN", (0, 0), (-1, -1), "MIDDLE"), ("LEFTPADDING", (0, 0), (-1, -1), 10), ("TOPPADDING", (0, 0), (-1, -1), 10), ("BOTTOMPADDING", (0, 0), (-1, -1), 10)]))
    story += [metrics, Spacer(1, 28 * mm), p("Disiapkan untuk WA Blast Source Code<br/>Versi 1.0 - 19 Juli 2026", "CoverSub")]

    section(story, "Ringkasan Eksekutif", "Menjual ketenangan operasional, bukan menjual ulang source code")
    story += [p("Pelanggan telah menggunakan aplikasi dan memperoleh support standar. Penawaran bulanan harus memberikan nilai baru yang terpisah: pemantauan, backup, bantuan update, health check, dan penanganan operasional prioritas.", "Lead"), callout("<b>Rekomendasi utama:</b> luncurkan <b>WA Blast Managed Care</b> seharga Rp199.000 per bulan, diawali program Founding Member Rp149.000 per bulan untuk 20 pelanggan lama.", True), Spacer(1, 4 * mm), cards([("Produk utama", "Managed Care: layanan pengelolaan ringan di atas server pelanggan."), ("Janji utama", "Pelanggan lebih tenang dan tidak perlu mengecek server sendirian."), ("Peluncuran", "Layani manual, validasi permintaan, kemudian otomasi."), ("Tujuan 90 hari", "Dapatkan 20 trial dan minimal 5 pelanggan berbayar.")])]
    story += [p("Prinsip yang tidak boleh dilanggar", "H2x")] + bullets(["Aplikasi dan server yang telah disetup tetap dapat digunakan ketika Managed Care dihentikan.", "Setiap bulan pelanggan menerima bukti nilai: laporan, status backup, temuan, dan pekerjaan.", "Support standar menjelaskan cara penggunaan; Managed Care membantu mengerjakan operasional.", "Jangan menjanjikan bebas blokir, uptime mutlak, atau penyelesaian semua gangguan tanpa batas."])

    section(story, "Arsitektur Penawaran", "Pisahkan lisensi, support standar, dan layanan Managed")
    story += [data_table(["Komponen", "Model Bayar", "Hak / Manfaat", "Fungsi"], [
        ["<b>Setup Aplikasi</b>", "Sekali beli", "Jasa instalasi server dan aplikasi siap pakai", "Pendapatan akuisisi"],
        ["<b>Update & Support</b>", "12 bulan lalu renewal", "Update produk, bug fix, dokumentasi, support standar", "Pendapatan maintenance"],
        ["<b>Managed Care</b>", "Bulanan/tahunan", "Monitoring, backup, health check, bantuan update dan prioritas", "Pendapatan berulang"],
        ["<b>Custom Work</b>", "Per proyek", "Integrasi, fitur custom, migrasi besar", "Pendapatan jasa"],
    ], [35 * mm, 32 * mm, 78 * mm, 35 * mm]), p("Perbedaan layanan", "H2x"), cards([("Support standar", "Menjawab pertanyaan, menangani bug produk, memberi panduan, dan mengikuti antrean normal."), ("Managed Care", "Memeriksa instalasi, backup, log, memasang update, dan memberi jalur prioritas.")]), p("Tidak termasuk Managed Care", "H2x")] + bullets(["Pembuatan fitur baru dan integrasi custom.", "Biaya VPS, domain, API pihak ketiga, atau penyimpanan.", "Migrasi besar dan operasional 24/7.", "Jaminan akun tidak diblokir oleh platform."])

    section(story, "Target & Positioning", "Dahulukan pelanggan yang merasakan kerepotan operasional")
    story += [cards([("Owner bisnis", "Tidak memahami server dan tidak punya staf teknis. Pesan: Fokus jualan, teknis WA Blast kami bantu pantau."), ("Operator internal", "Kesulitan membaca error dan update. Pesan: Ada pendamping teknis ketika tim membutuhkan bantuan."), ("Agency/reseller", "Menangani banyak instalasi. Pesan: Kelola lebih banyak klien tanpa menambah beban teknis sendiri.")], 3), p("Prioritas prospek pelanggan lama", "H2x"), data_table(["Prioritas", "Ciri Prospek", "Pendekatan"], [
        ["Tinggi", "Pernah meminta instalasi, mengalami service mati, atau sering bertanya update", "Undang personal ke trial"],
        ["Tinggi", "Menggunakan aplikasi untuk operasional bisnis harian", "Tawarkan monitoring dan backup"],
        ["Sedang", "Agency dengan beberapa klien", "Tawarkan Agency setelah Basic tervalidasi"],
        ["Rendah", "Menggunakan untuk belajar dan belum deploy", "Tawarkan deployment atau kelas"],
    ], [25 * mm, 95 * mm, 60 * mm]), callout("<b>Positioning:</b> WA Blast Managed Care adalah pendamping operasional untuk pemilik server yang ingin aplikasi terpantau, memiliki backup, mendapat bantuan update, dan memperoleh respons prioritas tanpa mengubah kepemilikan server mereka.")]

    section(story, "Paket & Harga", "Mulai dengan satu paket utama agar keputusan mudah")
    story += [cards([("Managed Care - Rp199.000/bulan", "Atau Rp1.990.000/tahun. Health check bulanan, pemeriksaan backup, bantuan update, pemeriksaan log ringan, priority support, satu bantuan langsung, dan laporan."), ("Founding Member - Rp149.000/bulan", "Harga terkunci selama langganan aktif. Trial 30 hari, maksimal 20 pelanggan lama, semua manfaat Managed Care, dan wajib memberi feedback.")]), p("Paket lanjutan", "H2x"), callout("<b>Managed Agency - Rp599.000/bulan:</b> hingga lima instalasi klien, bantuan deployment, prioritas lebih tinggi, dan laporan per instalasi."), p("Ketentuan komersial", "H2x")] + bullets(["Paket dapat dihentikan; server dan aplikasi tetap dapat digunakan.", "Paket tahunan setara pembayaran 10 bulan.", "Pekerjaan di luar cakupan diberikan estimasi terpisah.", "Priority support berarti prioritas antrean, bukan jaminan semua masalah langsung selesai.", "Harga Founding Member berlaku selama pembayaran tidak terputus."])

    section(story, "Funnel 30 Hari", "Biarkan pelanggan merasakan nilai sebelum diminta membayar")
    funnel = [
        ["Hari 0", "Undangan personal kepada pelanggan aktif; tawarkan trial dan kuota Founding Member."],
        ["Hari 1-3", "Onboarding: catat server, versi aplikasi, PIC, metode backup, dan kanal komunikasi."],
        ["Hari 3-7", "First value: health check awal, backup pertama, serta temuan singkat."],
        ["Hari 14", "Progress update: pekerjaan, perbaikan, dan rekomendasi berikutnya."],
        ["Hari 21", "Penawaran konversi berdasarkan nilai yang telah diterima."],
        ["Hari 27", "Reminder tanggal trial berakhir dan pilihan pembayaran."],
        ["Hari 30", "Konversi atau offboarding yang baik tanpa mengunci aplikasi."],
    ]
    story += [data_table(["Waktu", "Aktivitas"], funnel, [30 * mm, 150 * mm]), p("Saluran pemasaran", "H2x"), data_table(["Saluran", "Peran", "Frekuensi"], [
        ["WhatsApp personal", "Undangan prioritas dan follow-up trial", "Sesuai funnel"], ["Grup member", "Edukasi dan pengumuman batch", "1-2 kali/minggu"], ["Dashboard", "Status, laporan, dan tombol berlangganan", "Selalu tersedia"], ["Email", "Laporan dan pembayaran", "Bulanan"], ["Klinik", "Demonstrasi keahlian dan prospek", "Bulanan"]
    ], [43 * mm, 92 * mm, 45 * mm]), callout("Jangan mengirim promosi yang sama kepada semua pembeli. Prioritaskan pengguna aktif dan personalisasikan undangan berdasarkan masalah yang pernah mereka alami.")]

    section(story, "Copy Marketing", "Naskah siap pakai untuk pelanggan lama")
    copies = [
        ("Undangan", "Halo Kak {{nama}}, kami membuka 20 kuota WA Blast Managed Care khusus pengguna aktif. Selama 30 hari kami membantu health check, backup, pengecekan error, dan update. Trial gratis dan tidak mengubah kepemilikan server. Setelah trial bisa lanjut Rp149.000/bulan atau berhenti tanpa membuat aplikasi terkunci. Apakah ingin masuk batch pertama?"),
        ("Laporan minggu pertama", "Pemeriksaan awal sudah selesai. Minggu ini kami memeriksa {{jumlah_pemeriksaan}} bagian, memvalidasi {{jumlah_backup}} backup, dan menemukan {{jumlah_temuan}} hal yang perlu diperhatikan. Ringkasannya kami lampirkan."),
        ("Konversi hari ke-21", "Selama trial kami sudah membantu {{hasil_utama}}. Jika ingin monitoring, backup, health check, bantuan update, dan priority support tetap berjalan, lanjutkan sebagai Founding Member Rp149.000/bulan."),
        ("Reminder", "Trial berakhir pada {{tanggal}}. Pilih Rp149.000/bulan atau Rp1.490.000/tahun. Jika belum ingin melanjutkan, aplikasi tetap bisa digunakan seperti biasa."),
    ]
    for title, text in copies:
        story += [p(title, "H2x"), callout(text)]
    story += [p("Headline landing page", "H2x"), callout("<b>WA Blast sudah Anda gunakan. Sekarang operasionalnya tidak perlu Anda urus sendirian.</b><br/>Monitoring, backup, bantuan update, health check, dan priority support untuk pengguna setia.", True)]

    section(story, "Penanganan Keberatan", "Jawaban sales yang transparan dan tidak defensif")
    story += [data_table(["Keberatan", "Jawaban"], [
        ["Bukannya sudah dapat support?", "Benar. Support standar tetap berlaku. Managed Care adalah pemeriksaan langsung, backup, bantuan update, monitoring, dan prioritas."],
        ["Kalau berhenti aplikasinya mati?", "Tidak. Server dan aplikasi tetap dapat digunakan; hanya layanan Managed yang berhenti."],
        ["Saya bisa mengurus sendiri.", "Tentu. Paket ini opsional untuk menghemat waktu atau mendapatkan pendamping teknis."],
        ["Kenapa bulanan?", "Pemeriksaan, backup, bantuan update, dan kesiapan bantuan diberikan berkelanjutan. Ada opsi tahunan."],
        ["Dijamin tidak diblokir?", "Tidak ada jaminan. Kami membantu pengurangan risiko, tetapi keputusan platform di luar kendali kami."],
        ["Bisa tambah fitur?", "Pengembangan custom tidak termasuk dan akan dibuatkan estimasi proyek terpisah."],
    ], [58 * mm, 122 * mm]), p("Batas layanan wajib tertulis", "H2x"), cards([("Jam layanan", "Contoh Senin-Jumat, 09.00-17.00 WIB."), ("Target respons", "Contoh maksimal empat jam kerja untuk prioritas."), ("Kuota bantuan", "Satu sesi remote maksimal 60 menit per bulan."), ("Keamanan", "Akses sementara, hak minimum, dan catatan perubahan.")])]

    section(story, "Operasional", "Pastikan pendapatan bulanan tidak habis oleh beban support")
    story += [p("SOP bulanan per pelanggan", "H2x")] + bullets(["Pemeriksaan service, koneksi, disk, versi, dan error penting.", "Cek jadwal backup, keberhasilan, lokasi penyimpanan, serta kemampuan restore dasar.", "Informasikan update dan bantu pemasangan sesuai jadwal.", "Catat permintaan, waktu respons, waktu pengerjaan, dan cakupan.", "Kirim laporan pekerjaan, temuan, risiko, dan rekomendasi."])
    story += [p("Kontrol margin", "H2x"), cards([("Catat waktu", "Hitung menit untuk setiap pelanggan dan jenis masalah."), ("Batasi cakupan", "Pisahkan support ringan, remote session, dan custom."), ("Otomasi", "Otomatiskan laporan dan notifikasi setelah proses tervalidasi.")], 3), callout("<b>Kapasitas awal:</b> batasi 20 Founding Member. Jika rata-rata membutuhkan lebih dari 45-60 menit pekerjaan aktif per pelanggan per bulan, evaluasi harga, cakupan, atau proses."), p("Keamanan akses", "H2x")] + bullets(["Minta persetujuan sebelum remote access atau perubahan.", "Gunakan akun teknis dengan hak minimum.", "Jangan menyimpan password permanen lewat chat.", "Catat perubahan dan sediakan prosedur pemulihan.", "Jelaskan tanggung jawab backup dalam syarat layanan."])

    section(story, "Target & Pengukuran", "Nilai keberhasilan dari pembayaran dan biaya pelayanan")
    story += [data_table(["Metrik", "Rumus", "Target uji"], [
        ["Trial acceptance", "Peserta trial / pelanggan diundang", "Min. 25%"], ["Trial activation", "Onboarding selesai / peserta trial", "Min. 70%"], ["Paid conversion", "Berbayar / trial aktif", "Min. 25%"], ["Waktu layanan", "Menit pelayanan / pelanggan", "Maks. 60 menit"], ["Retention", "Bertahan / pelanggan awal bulan", "Pantau 3 bulan"], ["Margin", "Pendapatan - waktu - tools", "Harus positif"]
    ], [52 * mm, 82 * mm, 46 * mm]), p("Simulasi MRR", "H2x"), data_table(["Skenario", "Komposisi", "MRR"], [
        ["Validasi", "5 x Rp149.000", "Rp745.000"], ["Batch penuh", "20 x Rp149.000", "Rp2.980.000"], ["Pertumbuhan", "20 Founding + 20 reguler", "Rp6.960.000"], ["Dengan Agency", "Pertumbuhan + 5 x Rp599.000", "Rp9.955.000"]
    ], [45 * mm, 85 * mm, 50 * mm]), p("Angka merupakan simulasi, bukan proyeksi yang dijamin.", "Small"), callout("<b>Gerbang investasi:</b> jangan membangun monitoring cloud besar sebelum terdapat pelanggan yang benar-benar membayar. Pembayaran adalah validasi yang lebih kuat daripada jawaban tertarik.", True)]

    section(story, "Rencana Eksekusi", "Roadmap 90 hari")
    story += [cards([("Hari 1-14: Persiapan", "Tetapkan cakupan, SOP, onboarding, daftar prospek, serta pembayaran."), ("Hari 15-45: Pilot", "Undang 20 member, beri first value, catat waktu, dan konversikan trial."), ("Hari 46-90: Perbaikan", "Evaluasi harga, ambil testimoni, otomasi, dan siapkan Agency.")], 3), p("Checklist sebelum promosi", "H2x")] + bullets(["Nama paket, harga, trial, dan kuota final.", "Perbedaan support standar dan Managed Care tertulis.", "Jam layanan, target respons, dan batas bantuan tersedia.", "Penghentian layanan tidak mematikan source code.", "SOP akses server dan kredensial tersedia.", "Template health check dan laporan siap.", "Pembayaran bulanan dan tahunan siap.", "Prospek telah disegmentasi.", "Copy undangan, reminder, dan konversi siap.", "Waktu pelayanan akan dicatat."])
    story += [Spacer(1, 3 * mm), callout("<b>Keputusan:</b> luncurkan pilot manual sekarang. Gunakan harga Founding Member Rp149.000/bulan, trial 30 hari, dan kuota 20 pelanggan. Pertahankan harga normal Rp199.000/bulan. Bangun otomasi setelah minimal lima pelanggan bersedia membayar.", True), Spacer(1, 4 * mm), p("Dokumen ini adalah rencana pemasaran dan operasional awal. Harga, cakupan, serta target perlu disesuaikan setelah data pilot tersedia.", "Small")]

    doc = SimpleDocTemplate(str(OUTPUT), pagesize=A4, rightMargin=15 * mm, leftMargin=15 * mm, topMargin=16 * mm, bottomMargin=18 * mm, title="Strategi Marketing WA Blast Managed Care", author="WA Blast")
    doc.build(story, onFirstPage=draw_page, onLaterPages=draw_page)
    print(OUTPUT)


if __name__ == "__main__":
    build()
