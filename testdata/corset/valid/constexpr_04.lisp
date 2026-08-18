(defcolumns (X :i16) (Y :i16))
;; X + 2 == Y + 2
(defconstraint c1 () (== 0 (- (+ X 2) (+ Y (* 2 1)))))
;; X + 4 == Y + 4
(defconstraint c2 () (== 0 (- (+ X 4) (+ Y (* 2 2)))))
;; X + 8 == Y + 8
(defconstraint c3 () (== 0 (- (+ X 8) (+ Y (* 2 2 2)))))
;; X + 16 == Y + 16
(defconstraint c4 () (== 0 (- (+ X 16) (+ Y (* 2 2 2 2)))))
;; X + 32 == Y + 32
(defconstraint c5 () (== 0 (- (+ X 32) (+ Y (* 2 2 2 2 2)))))
;; X + 64 == Y + 64
(defconstraint c6 () (== 0 (- (+ X 64) (+ Y (* 2 2 2 2 2 2)))))
;; X + 128 == Y + 128
(defconstraint c7 () (== 0 (- (+ X 128) (+ Y (* 2 2 2 2 2 2 2)))))
;; X + 256 == Y + 256
(defconstraint c8 () (== 0 (- (+ X 256) (+ Y (* 2 2 2 2 2 2 2 2)))))
