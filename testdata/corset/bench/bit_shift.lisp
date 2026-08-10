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

;; Combine bits into a nibble
(defpurefun (bits a1 a2 a3 a4)
  (+ (* 1 a1)
     (* 2 a2)
     (* 4 a3)
     (* 8 a4)))

;; For X
(defconstraint X_bits () (eq! X (bits x1 x2 x3 x4)))
;; For Y
(defconstraint Y_bits () (eq! Y (bits y1 y2 y3 y4)))
;; Relating X and Y
(defconstraint X_Y_bits_i () (eq!  0 y1))
(defconstraint X_Y_bits_ii () (eq! x1 y2))
(defconstraint X_Y_bits_iii () (eq! x2 y3))
(defconstraint X_Y_bits_iv () (eq! x3 y4))
