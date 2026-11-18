# Базовые компоненты (A-04)

Библиотека компонентов находится в Figma, на странице **Components**:
https://www.figma.com/design/eEmIWFxA3osoTxvca04MNs/Diploma?node-id=9-2&p=f&t=JQR2WimC2ZPNrcOh-0

Все цвета, отступы, радиусы и размеры шрифта привязаны к переменным из `design/tokens.json` (задача A-03).

## Компоненты

| Компонент | Варианты | Свойства |
|---|---|---|
| Button | Style: Primary, Secondary, Ghost, Danger; Size: Small, Medium; State: Default, Hover, Disabled (24) | Label |
| Input | State: Default, Focus, Error, Disabled (4) | Label, Placeholder, Helper, Show helper |
| Tab | State: Default, Hover, Active, Disabled (4) | Label |
| Tabs | панель из экземпляров Tab | — |
| Badge | Tone: Neutral, Info, Success, Warning, Error и 8 акцентов (13) | Label |
| Toast | Type: Info, Success, Warning, Error (4) | Title, Message |
| Modal | Type: Default, Destructive (2); кнопки футера — экземпляры Button | Title, Body |
| Skeleton | Shape: Text, Title, Avatar, Image, Button (5) | — |

Всего: 7 семейств компонентов, 57 вариантов.

## Текстовые стили

Text/Body, Text/Label, Text/Label Small, Text/Caption, Text/Title (шрифт Inter).

## Где используется

Кнопки и бейджи заменены экземплярами компонентов на экранах вайрфреймов (разделы H, L, T, P, I, K, M, N, O, X, мобильные и меню).

## Ограничения

- У Skeleton нет анимации мерцания (в Figma через API её не задать).
- Тёмной темы нет: на тарифе Starter доступен один режим переменных.
- Тень у Toast и Modal задана вручную, а не токеном.
- Не заменены на компоненты: калькулятор, кнопки-иконки, шаги онбординга, чипы ТГО, вкладки профиля, кнопка «Сбросить прогресс».
