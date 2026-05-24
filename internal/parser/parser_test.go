package parser

import (
	"testing"
	"time"
)

// singleItemReceipt mirrors the real ledongthuc/pdf output for cheque_17252280-0942-8f6c…
// (one item, no delivery). Key format details:
//   - item index "1." is on its own line
//   - price is on a separate "≡NNN,NN" line after the "qty x unit" line
const singleItemReceipt = `
Кассовый чек № 2238
11.05.2026 17:37
https://www.ozon.ru/
Интернет Решения, ООО
ИНН 7704217370
Вид налогообложения: ОСН
Приход
1.
Фантазийная декоративная картина маслом ручной работы "Ангел Феникс" в разных стилях
(без рамки) 40х50см
1 x 469,00
≡469,00
Без НДС
≡469,00
ИНН продавца: 000000000000
Putian Licheng District Liangxing Trading Co., Ltd.
ИТОГ
≡469,00
Без НДС
≡469,00
Полный расчет
Зачет предварительной оплаты
≡469,00
ФП
ФН:
7380440903211869
РН ККТ:
0009878214024962
ФД:
208843
ФПД:
1107204984
Сайт ФНС:
www.nalog.ru
`

// deliveryReceipt mirrors the real ledongthuc/pdf output for cheque_17252280-0976-6e49…
// (one regular item + Доставка service item).
const deliveryReceipt = `
Кассовый чек № 1014
23.05.2026 17:30
https://www.ozon.ru/
Интернет Решения, ООО
ИНН 7704217370
Вид налогообложения: ОСН
Приход
1.
Парафиновая смазка для цепи велосипеда MAX WAX Chain Wax 30мл
1 x 213,27
≡213,27
в т.ч. НДС 5/105
≡10,16
ИНН продавца: 594401597914
Дерендяев Максим Алексеевич, ИП
2.
Доставка
1 x 11,73
≡11,73
Без НДС
≡11,73
ИНН продавца: 611322055016
ИП Ертевцян Артем Артурович
ИТОГ
≡225,00
Без НДС
≡11,73
в т.ч. НДС 5/105
≡10,16
Предварительная оплата
Безналичными
≡225,00
ФП
ФН:
7380440903143771
РН ККТ:
0009381560063678
ФД:
93747
ФПД:
3949258726
Сайт ФНС:
www.nalog.ru
`

// correctionReceipt mirrors the real ledongthuc/pdf output for cheque_8b669424…
// (correction receipt with "Возврат прихода" section marker instead of "Приход").
const correctionReceipt = `
Кассовый чек коррекции № 1172
20.05.2026 01:49
https://www.ozon.ru/
Интернет Решения, ООО
ИНН 7704217370
Вид налогообложения: ОСН
Возврат прихода
1.
Перчатки GLACC
1 x 767,00
≡767,00
в т.ч. НДС 20%
≡127,83
ИНН продавца: 000000000000
Прокушенков Дмитрий Сергеевич, ИП
ИТОГ
≡767,00
в т.ч. НДС 20%
≡127,83
Полный расчет
Безналичными
≡767,00
ФП
ФН:
7380440903216680
Тип операции:
Самостоятельная операция
Дата корректируемого расчета:
19.11.2023
РН ККТ:
0009613842030788
ФД:
66033
ФПД:
1736911978
Сайт ФНС:
www.nalog.ru
`

func TestParseReceipt_SingleItem(t *testing.T) {
	r, err := ParseReceipt(singleItemReceipt)
	if err != nil {
		t.Fatalf("ParseReceipt error: %v", err)
	}

	wantTime := time.Date(2026, 5, 11, 17, 37, 0, 0, time.UTC)
	if !r.DateTime.Equal(wantTime) {
		t.Errorf("DateTime = %v, want %v", r.DateTime, wantTime)
	}

	if len(r.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(r.Items))
	}

	item := r.Items[0]
	wantName := `Фантазийная декоративная картина маслом ручной работы "Ангел Феникс" в разных стилях (без рамки) 40х50см`
	if item.Name != wantName {
		t.Errorf("Name = %q, want %q", item.Name, wantName)
	}
	if item.Price != 469.00 {
		t.Errorf("Price = %v, want 469.00", item.Price)
	}
	if item.Service {
		t.Error("Service = true, want false")
	}

	rows := DistributeDelivery(r.Items)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Price != 469.00 {
		t.Errorf("row Price = %v, want 469.00", rows[0].Price)
	}
}

func TestParseReceipt_WithDelivery(t *testing.T) {
	r, err := ParseReceipt(deliveryReceipt)
	if err != nil {
		t.Fatalf("ParseReceipt error: %v", err)
	}

	wantTime := time.Date(2026, 5, 23, 17, 30, 0, 0, time.UTC)
	if !r.DateTime.Equal(wantTime) {
		t.Errorf("DateTime = %v, want %v", r.DateTime, wantTime)
	}

	if len(r.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(r.Items))
	}

	if r.Items[0].Service {
		t.Error("Items[0].Service = true, want false (regular item)")
	}
	if !r.Items[1].Service {
		t.Error("Items[1].Service = false, want true (Доставка)")
	}

	rows := DistributeDelivery(r.Items)
	// Delivery (11.73) is fully proportional to the single regular item (213.27),
	// so the entire delivery cost is added to it: 213.27 + 11.73 = 225.00.
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Price != 225.00 {
		t.Errorf("row Price = %v, want 225.00", rows[0].Price)
	}
}

func TestParseReceipt_CorrectionReceipt(t *testing.T) {
	r, err := ParseReceipt(correctionReceipt)
	if err != nil {
		t.Fatalf("ParseReceipt error: %v", err)
	}

	wantTime := time.Date(2026, 5, 20, 1, 49, 0, 0, time.UTC)
	if !r.DateTime.Equal(wantTime) {
		t.Errorf("DateTime = %v, want %v", r.DateTime, wantTime)
	}

	if len(r.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(r.Items))
	}

	item := r.Items[0]
	if item.Name != "Перчатки GLACC" {
		t.Errorf("Name = %q, want %q", item.Name, "Перчатки GLACC")
	}
	if item.Price != 767.00 {
		t.Errorf("Price = %v, want 767.00", item.Price)
	}

	rows := DistributeDelivery(r.Items)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Price != 767.00 {
		t.Errorf("row Price = %v, want 767.00", rows[0].Price)
	}
}
