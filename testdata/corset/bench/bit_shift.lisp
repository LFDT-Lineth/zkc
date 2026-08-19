(defcolumns
  (X :i4) (Y :i4)
  ;; X bits
  (x1 :binary@prove)
  (x2 :binary@prove)
  (x3 :binary@prove)
  (x4 :binary@prove)
  ;; Y bits
  (y1 :binary@prove)
  (y2 :binary@prove)
  (y3 :binary@prove)
  (y4 :binary@prove))

;; For X
(defconstraint X_bits () (== X (+ (* 1 x1) (* 2 x2) (* 4 x3) (* 8 x4))))
;; For Y
(defconstraint Y_bits () (== Y (+ (* 1 y1) (* 2 y2) (* 4 y3) (* 8 y4))))
;; Relating X and Y
(defconstraint X_Y_bits_i () (==  0 y1))
(defconstraint X_Y_bits_ii () (== x1 y2))
(defconstraint X_Y_bits_iii () (== x2 y3))
(defconstraint X_Y_bits_iv () (== x3 y4))
