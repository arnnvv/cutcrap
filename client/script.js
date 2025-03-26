const processButton = document.getElementById('process-button');
const textInput = document.getElementById('text-input');
const ratioInput = document.getElementById('ratio-input');
const ratioValue = document.getElementById('ratio-value');
const downloadLink = document.getElementById('download-link');
const loadingIndicator = document.getElementById('loading-indicator');
const fileInput = document.getElementById('file-input');
const fileName = document.getElementById('file-name');
const resultCard = document.getElementById('result-card');
const originalSize = document.getElementById('original-size');
const processedSize = document.getElementById('processed-size');
const reductionPercent = document.getElementById('reduction-percent');

// Set your backend URL here
const baseUrl = 'http://localhost:8080';

// Update ratio value display
ratioInput.addEventListener('input', () => {
    ratioValue.textContent = ratioInput.value;
});

// Display file name when selected
fileInput.addEventListener('change', async (event) => {
    const file = event.target.files[0];
    if (file) {
        fileName.textContent = file.name;

        if (file.type === 'application/pdf') {
            try {
                // Show loading state for PDF processing
                fileName.textContent = `Processing ${file.name}...`;

                // Initialize PDF.js
                const pdfjsLib = window['pdfjs-dist/build/pdf'];
                pdfjsLib.GlobalWorkerOptions.workerSrc = `https://cdnjs.cloudflare.com/ajax/libs/pdf.js/${pdfjsLib.version}/pdf.worker.js`;

                const fileReader = new FileReader();
                fileReader.onload = async () => {
                    try {
                        const typedarray = new Uint8Array(fileReader.result);
                        const pdf = await pdfjsLib.getDocument(typedarray).promise;
                        let text = '';

                        // Process each page
                        for (let pageNum = 1; pageNum <= pdf.numPages; pageNum++) {
                            fileName.textContent = `Processing page ${pageNum}/${pdf.numPages}...`;
                            const page = await pdf.getPage(pageNum);
                            const content = await page.getTextContent();
                            const pageText = content.items.map(item => item.str).join(' ');
                            text += pageText + '\n';
                        }

                        textInput.value = text;
                        fileName.textContent = `${file.name} (${formatBytes(text.length)})`;
                    } catch (error) {
                        console.error('Error parsing PDF:', error);
                        fileName.textContent = `Error: Could not process ${file.name}`;
                        alert('Failed to parse PDF file. Please try another file.');
                    }
                };
                fileReader.readAsArrayBuffer(file);
            } catch (error) {
                console.error('PDF processing error:', error);
                fileName.textContent = `Error: Could not load ${file.name}`;
                alert('Error reading PDF file. PDF.js library might not be loaded correctly.');
            }
        } else if (file.type === 'text/plain') {
            const fileReader = new FileReader();
            fileReader.onload = (event) => {
                textInput.value = event.target.result;
                fileName.textContent = `${file.name} (${formatBytes(event.target.result.length)})`;
            };
            fileReader.readAsText(file);
        }
    }
});

// Format bytes to human-readable format
function formatBytes(bytes, decimals = 2) {
    if (bytes === 0) return '0 Bytes';

    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];

    const i = Math.floor(Math.log(bytes) / Math.log(k));

    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

// Get selected processing mode
function getSelectedMode() {
    return document.querySelector('input[name="mode"]:checked').value;
}

processButton.addEventListener('click', async () => {
    const text = textInput.value;
    const ratio = ratioInput.value;
    const mode = getSelectedMode();

    // Debug logging
    console.log("Selected mode:", mode);
    console.log("Radio buttons:", {
        document: document.getElementById('mode-document').checked,
        transcript: document.getElementById('mode-transcript').checked
    });

    // Input validation
    if (!text) {
        alert('Please enter text to process.');
        return;
    }

    if (!ratio || ratio < 0.1 || ratio > 1.0) {
        alert('Please enter a compression ratio between 0.1 and 1.0');
        return;
    }

    // Show loading state
    loadingIndicator.style.display = 'flex';
    processButton.disabled = true;

    const formData = new URLSearchParams();
    formData.append('text', text);
    formData.append('ratio', ratio);
    formData.append('mode', mode);

    try {
        console.log("Sending form data:", formData.toString());
        const response = await fetch(`${baseUrl}/process`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/x-www-form-urlencoded',
            },
            body: formData.toString(),
        });

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        // Get text content
        const processedText = await response.text();

        // Create blob from text content
        const blob = new Blob([processedText], {type: 'text/plain'});

        // Create download URL
        const url = window.URL.createObjectURL(blob);
        downloadLink.href = url;

        // Show result card and update stats
        resultCard.style.display = 'block';
        originalSize.textContent = formatBytes(text.length);
        processedSize.textContent = formatBytes(processedText.length);

        const reduction = ((text.length - processedText.length) / text.length * 100).toFixed(1);
        reductionPercent.textContent = `${reduction}%`;

        // Set appropriate filename for download based on mode
        const filename = mode === 'transcript' ? 'processed_transcript.txt' : 'processed.txt';
        downloadLink.setAttribute('download', filename);

    } catch (error) {
        console.error('Error processing text:', error);
        alert(`An error occurred during processing: ${error.message}`);
    } finally {
        loadingIndicator.style.display = 'none';
        processButton.disabled = false;
    }
});

// Initialize UI
window.addEventListener('DOMContentLoaded', () => {
    ratioValue.textContent = ratioInput.value;

    // Add tooltip for transcript mode
    const transcriptOption = document.querySelector('input[value="transcript"]').parentElement;
    transcriptOption.title = "Use this mode for podcast transcripts, interviews, or dialogue-heavy content";
});
