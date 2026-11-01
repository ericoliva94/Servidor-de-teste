let currentIndex = 0;

function rotateCarousel() {
    const track = document.querySelector(".carousel-track");
    const items = document.querySelectorAll(".store-item");
    const itemsPerPage = 4;

    const maxIndex = Math.ceil(items.length / itemsPerPage) - 1;

    currentIndex = (currentIndex + 1) > maxIndex ? 0 : currentIndex + 1;

    track.style.transform = `translateX(-${currentIndex * 100}%)`;
}

setInterval(rotateCarousel, 7000);
